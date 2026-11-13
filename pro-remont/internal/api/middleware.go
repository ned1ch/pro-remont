package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"pro-remont/internal/model"
)

type ctxKey string

const (
	userKey      ctxKey = "user"
	requestIDKey ctxKey = "request_id"
)

func (s *Server) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := randomID()
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		s.Logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"bytes", rec.bytes,
			"duration_ms", time.Since(start).Milliseconds(),
			"ip", clientIP(r),
			"request_id", requestID(r),
		)
	})
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	const csp = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
		"img-src 'self' data:; font-src 'self' data:; connect-src 'self'; media-src 'self'; " +
		"object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		h.Set("Content-Security-Policy", csp)
		next.ServeHTTP(w, r)
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.Logger.Error("panic recovered", "panic", rec, "path", r.URL.Path, "stack", string(debug.Stack()))
				if strings.HasPrefix(r.URL.Path, "/api/") {
					writeError(w, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка сервера")
				} else {
					http.Error(w, "Внутренняя ошибка сервера", http.StatusInternalServerError)
				}
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withUser(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, err := s.authenticate(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Требуется авторизация")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	}
}

func (s *Server) withPerm(perm string, next http.HandlerFunc) http.HandlerFunc {
	return s.withUser(func(w http.ResponseWriter, r *http.Request) {
		u := currentUser(r)
		if !model.HasPermission(u, perm) {
			writeError(w, http.StatusForbidden, "forbidden", "Недостаточно прав для этой операции")
			return
		}
		next(w, r)
	})
}

func (s *Server) withRole(role string, next http.HandlerFunc) http.HandlerFunc {
	return s.withUser(func(w http.ResponseWriter, r *http.Request) {
		u := currentUser(r)
		if u.Role != role && u.Role != model.RoleAdmin {
			writeError(w, http.StatusForbidden, "forbidden", "Недостаточно прав для этой операции")
			return
		}
		next(w, r)
	})
}

// optionalUser возвращает пользователя, если передан корректный токен, иначе nil.
func (s *Server) optionalUser(r *http.Request) *model.User {
	token := s.tokenFromRequest(r)
	if token == "" {
		return nil
	}
	id, err := s.Tokens.Parse(token)
	if err != nil {
		return nil
	}
	u, err := s.loadUser(r.Context(), id)
	if err != nil {
		return nil
	}
	return u
}

func (s *Server) authenticate(r *http.Request) (*model.User, error) {
	token := s.tokenFromRequest(r)
	if token == "" {
		return nil, errors.New("no token")
	}
	id, err := s.Tokens.Parse(token)
	if err != nil {
		return nil, err
	}
	u, err := s.loadUser(r.Context(), id)
	if err != nil {
		return nil, err
	}
	if !u.IsActive {
		return nil, errors.New("user is blocked")
	}
	return u, nil
}

func (s *Server) loadUser(ctx context.Context, id int64) (*model.User, error) {
	var u model.User
	err := s.DB.QueryRow(ctx, `
		SELECT id, COALESCE(email, ''), COALESCE(phone, ''), full_name, address, role::text,
		       permissions, is_active, created_at
		FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Email, &u.Phone, &u.FullName, &u.Address, &u.Role, &u.Permissions, &u.IsActive, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	if u.Permissions == nil {
		u.Permissions = []string{}
	}
	return &u, nil
}

func (s *Server) tokenFromRequest(r *http.Request) string {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		return c.Value
	}
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	return ""
}

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, token string, ttl time.Duration) {
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(ttl),
		MaxAge:   int(ttl.Seconds()),
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
	})
}

func currentUser(r *http.Request) *model.User {
	u, _ := r.Context().Value(userKey).(*model.User)
	return u
}

func requestID(r *http.Request) string {
	id, _ := r.Context().Value(requestIDKey).(string)
	return id
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func randomID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "req"
	}
	return hex.EncodeToString(b[:])
}

func contextWithTimeout(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), d)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

func isNotFound(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

func logDBError(logger *slog.Logger, r *http.Request, err error) {
	logger.Error("database error", "path", r.URL.Path, "request_id", requestID(r), "error", err)
}

type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	max      int
	window   time.Duration
}

func newLoginLimiter(max int, window time.Duration) *loginLimiter {
	return &loginLimiter{attempts: make(map[string][]time.Time), max: max, window: window}
}

func (l *loginLimiter) allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-l.window)
	list := l.attempts[key][:0]
	for _, t := range l.attempts[key] {
		if t.After(cutoff) {
			list = append(list, t)
		}
	}
	l.attempts[key] = list

	if len(list) >= l.max {
		return false, list[0].Add(l.window).Sub(now)
	}
	l.attempts[key] = append(l.attempts[key], now)
	return true, 0
}

func (l *loginLimiter) reset(key string) {
	l.mu.Lock()
	delete(l.attempts, key)
	l.mu.Unlock()
}
