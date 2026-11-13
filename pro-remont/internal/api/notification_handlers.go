package api

import (
	"net/http"
	"time"

	"pro-remont/internal/model"
)

func (s *Server) handleNotifications(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()

	limit := 50
	if r.URL.Query().Get("limit") != "" {
		_, l, _ := pageParams(r)
		limit = l
	}
	rows, err := s.DB.Query(ctx, `
		SELECT id, type, title, message, request_id, is_read, created_at
		FROM notifications WHERE user_id = $1
		ORDER BY is_read, created_at DESC
		LIMIT $2`, u.ID, limit)
	if err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось загрузить уведомления")
		return
	}
	defer rows.Close()

	items := make([]model.Notification, 0, limit)
	for rows.Next() {
		var n model.Notification
		if err := rows.Scan(&n.ID, &n.Type, &n.Title, &n.Message, &n.RequestID, &n.IsRead, &n.CreatedAt); err != nil {
			logDBError(s.Logger, r, err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Ошибка чтения уведомлений")
			return
		}
		items = append(items, n)
	}
	var unread int
	_ = s.DB.QueryRow(ctx,
		`SELECT count(*) FROM notifications WHERE user_id = $1 AND NOT is_read`, u.ID).Scan(&unread)
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "unread": unread})
}

func (s *Server) handleReadNotification(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "Некорректный идентификатор")
		return
	}
	u := currentUser(r)
	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()

	ct, err := s.DB.Exec(ctx, `
		UPDATE notifications SET is_read = TRUE WHERE id = $1 AND user_id = $2`, id, u.ID)
	if err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось обновить уведомление")
		return
	}
	if ct.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_found", "Уведомление не найдено")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleReadAllNotifications(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()

	if _, err := s.DB.Exec(ctx, `
		UPDATE notifications SET is_read = TRUE WHERE user_id = $1 AND NOT is_read`, u.ID); err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось обновить уведомления")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
