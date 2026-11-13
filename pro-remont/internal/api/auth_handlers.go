package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"pro-remont/internal/auth"
	"pro-remont/internal/model"
)

type registerInput struct {
	FullName string `json:"full_name"`
	Contact  string `json:"contact"`
	Password string `json:"password"`
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var in registerInput
	if !decodeJSON(w, r, &in) {
		return
	}

	fields := map[string]string{}
	name := cleanString(in.FullName, 120)
	if len([]rune(name)) < 2 {
		fields["full_name"] = "Укажите имя (минимум 2 символа)"
	}
	contact := strings.TrimSpace(in.Contact)
	var email, phone string
	if auth.IsEmail(contact) {
		email, _ = auth.NormalizeEmail(contact)
	} else if p, ok := auth.NormalizePhone(contact); ok {
		phone = p
	} else {
		fields["contact"] = "Введите корректный email или телефон"
	}
	if len([]rune(in.Password)) < 6 {
		fields["password"] = "Пароль должен содержать минимум 6 символов"
	}
	if len(fields) > 0 {
		writeFieldError(w, fields)
		return
	}

	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()

	if email != "" {
		var exists bool
		if err := s.DB.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM users WHERE lower(email) = lower($1))`, email).Scan(&exists); err != nil {
			logDBError(s.Logger, r, err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось проверить email")
			return
		}
		if exists {
			writeFieldError(w, map[string]string{"contact": "Пользователь с таким email уже зарегистрирован"})
			return
		}
	}
	if phone != "" {
		var exists bool
		if err := s.DB.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM users WHERE phone = $1)`, phone).Scan(&exists); err != nil {
			logDBError(s.Logger, r, err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось проверить телефон")
			return
		}
		if exists {
			writeFieldError(w, map[string]string{"contact": "Пользователь с таким телефоном уже зарегистрирован"})
			return
		}
	}

	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, "weak_password", "Слишком короткий пароль")
		return
	}

	var emailArg, phoneArg any
	if email != "" {
		emailArg = email
	}
	if phone != "" {
		phoneArg = phone
	}

	var id int64
	var createdAt time.Time
	err = s.DB.QueryRow(ctx, `
		INSERT INTO users (email, phone, full_name, password_hash, role)
		VALUES ($1, $2, $3, $4, 'client'::user_role)
		RETURNING id, created_at`,
		emailArg, phoneArg, name, hash).Scan(&id, &createdAt)
	if err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось создать аккаунт")
		return
	}

	token, err := s.Tokens.Issue(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось создать сессию")
		return
	}
	s.setSessionCookie(w, r, token, s.Tokens.TTL())

	writeJSON(w, http.StatusCreated, map[string]any{"user": model.User{
		ID: id, Email: email, Phone: phone, FullName: name,
		Role: model.RoleClient, Permissions: []string{}, IsActive: true, CreatedAt: createdAt,
	}})
}

type loginInput struct {
	Contact  string `json:"contact"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var in loginInput
	if !decodeJSON(w, r, &in) {
		return
	}

	ip := clientIP(r)
	if ok, retry := s.loginLimiter.allow(ip); !ok {
		retrySec := int(retry.Seconds()) + 1
		w.Header().Set("Retry-After", strconv.Itoa(retrySec))
		writeError(w, http.StatusTooManyRequests, "too_many_attempts", "Слишком много попыток входа. Попробуйте позже.")
		return
	}

	contact := strings.TrimSpace(in.Contact)
	if contact == "" || in.Password == "" {
		writeFieldError(w, map[string]string{
			"contact":  "Введите email или телефон",
			"password": "Введите пароль",
		})
		return
	}

	var email, phone string
	if e, ok := auth.NormalizeEmail(contact); ok {
		email = e
	} else if p, ok := auth.NormalizePhone(contact); ok {
		phone = p
	} else {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "Неверный email/телефон или пароль")
		return
	}

	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()

	var (
		id       int64
		hash     string
		isActive bool
		fullName string
		role     string
	)
	err := s.DB.QueryRow(ctx, `
		SELECT id, password_hash, is_active, full_name, role::text
		FROM users
		WHERE ($1 <> '' AND lower(email) = lower($1)) OR ($2 <> '' AND phone = $2)
		LIMIT 1`, email, phone).Scan(&id, &hash, &isActive, &fullName, &role)
	if err != nil || !auth.CheckPassword(hash, in.Password) {
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			logDBError(s.Logger, r, err)
		}
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "Неверный email/телефон или пароль")
		return
	}
	if !isActive {
		writeError(w, http.StatusForbidden, "account_disabled", "Аккаунт заблокирован. Обратитесь к администратору.")
		return
	}

	s.loginLimiter.reset(ip)

	token, err := s.Tokens.Issue(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось создать сессию")
		return
	}
	s.setSessionCookie(w, r, token, s.Tokens.TTL())

	u, err := s.loadUser(ctx, id)
	if err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось загрузить профиль")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": u})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	// Публичный эндпоинт: для гостя возвращаем user=null, чтобы фронтенд
	// мог проверить сессию без ошибки 401 в консоли браузера.
	u := s.optionalUser(r)
	writeJSON(w, http.StatusOK, map[string]any{"user": u})
}

func (s *Server) handleGetProfile(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"user": currentUser(r)})
}

type profileInput struct {
	FullName string `json:"full_name"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Address  string `json:"address"`
}

func (s *Server) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	var in profileInput
	if !decodeJSON(w, r, &in) {
		return
	}
	u := currentUser(r)

	fields := map[string]string{}
	name := cleanString(in.FullName, 120)
	if len([]rune(name)) < 2 {
		fields["full_name"] = "Укажите имя (минимум 2 символа)"
	}
	var email, phone string
	if strings.TrimSpace(in.Email) != "" {
		if e, ok := auth.NormalizeEmail(in.Email); ok {
			email = e
		} else {
			fields["email"] = "Некорректный email"
		}
	}
	if strings.TrimSpace(in.Phone) != "" {
		if p, ok := auth.NormalizePhone(in.Phone); ok {
			phone = p
		} else {
			fields["phone"] = "Некорректный телефон"
		}
	}
	if email == "" && phone == "" {
		fields["email"] = "Укажите email или телефон"
	}
	address := cleanString(in.Address, 255)
	if len(fields) > 0 {
		writeFieldError(w, fields)
		return
	}

	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()

	if email != "" {
		var exists bool
		if err := s.DB.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM users WHERE lower(email) = lower($1) AND id <> $2)`,
			email, u.ID).Scan(&exists); err != nil {
			logDBError(s.Logger, r, err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось проверить email")
			return
		}
		if exists {
			writeFieldError(w, map[string]string{"email": "Этот email уже используется"})
			return
		}
	}
	if phone != "" {
		var exists bool
		if err := s.DB.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM users WHERE phone = $1 AND id <> $2)`,
			phone, u.ID).Scan(&exists); err != nil {
			logDBError(s.Logger, r, err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось проверить телефон")
			return
		}
		if exists {
			writeFieldError(w, map[string]string{"phone": "Этот телефон уже используется"})
			return
		}
	}

	var emailArg, phoneArg any
	if email != "" {
		emailArg = email
	}
	if phone != "" {
		phoneArg = phone
	}
	_, err := s.DB.Exec(ctx, `
		UPDATE users SET full_name = $1, email = $2, phone = $3, address = $4
		WHERE id = $5`, name, emailArg, phoneArg, address, u.ID)
	if err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось сохранить профиль")
		return
	}

	updated, err := s.loadUser(ctx, u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось загрузить профиль")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": updated})
}
