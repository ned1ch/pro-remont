package api

import (
	"encoding/json"
	"net/http"
	"time"

	"pro-remont/internal/auth"
)

const draftCookie = "pro_draft"

type requisites struct {
	CustomerName string `json:"customer_name"`
	Phone        string `json:"phone"`
	Email        string `json:"email"`
	Address      string `json:"address"`
}

func (s *Server) handleSaveRequisites(w http.ResponseWriter, r *http.Request) {
	var in requisites
	if !decodeJSON(w, r, &in) {
		return
	}
	fields := map[string]string{}
	name := cleanString(in.CustomerName, 120)
	if len([]rune(name)) < 2 {
		fields["customer_name"] = "Укажите имя"
	}
	phone, ok := auth.NormalizePhone(in.Phone)
	if !ok {
		fields["phone"] = "Некорректный телефон"
	}
	address := cleanString(in.Address, 255)
	if len([]rune(address)) < 5 {
		fields["address"] = "Укажите адрес (минимум 5 символов)"
	}
	var email string
	if in.Email != "" {
		if e, valid := auth.NormalizeEmail(in.Email); valid {
			email = e
		} else {
			fields["email"] = "Некорректный email"
		}
	}
	if len(fields) > 0 {
		writeFieldError(w, fields)
		return
	}

	out := requisites{CustomerName: name, Phone: phone, Email: email, Address: address}
	data, err := json.Marshal(out)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось сохранить черновик")
		return
	}

	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()

	sid := ""
	if c, err := r.Cookie(draftCookie); err == nil {
		sid = c.Value
	}
	if sid == "" {
		sid = randomID()
	}
	_, err = s.DB.Exec(ctx, `
		INSERT INTO session_drafts (sid, data, expires_at)
		VALUES ($1, $2::jsonb, now() + interval '24 hours')
		ON CONFLICT (sid) DO UPDATE SET data = EXCLUDED.data, expires_at = EXCLUDED.expires_at`,
		sid, string(data))
	if err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось сохранить черновик")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     draftCookie,
		Value:    sid,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
		Expires:  time.Now().Add(24 * time.Hour),
	})
	writeJSON(w, http.StatusOK, map[string]any{"saved": true, "requisites": out})
}

func (s *Server) handleGetRequisites(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(draftCookie)
	if err != nil || c.Value == "" {
		writeJSON(w, http.StatusOK, map[string]any{"saved": false})
		return
	}

	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()

	var raw []byte
	err = s.DB.QueryRow(ctx, `
		SELECT data FROM session_drafts
		WHERE sid = $1 AND expires_at > now()`, c.Value).Scan(&raw)
	if err != nil {
		if !isNotFound(err) {
			logDBError(s.Logger, r, err)
		}
		writeJSON(w, http.StatusOK, map[string]any{"saved": false})
		return
	}
	var out requisites
	if err := json.Unmarshal(raw, &out); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"saved": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"saved": true, "requisites": out})
}
