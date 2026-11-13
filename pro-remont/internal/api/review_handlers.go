package api

import (
	"net/http"
	"time"

	"pro-remont/internal/model"
)

func scanReview(sc rowScanner) (model.Review, error) {
	var rv model.Review
	err := sc.Scan(&rv.ID, &rv.RequestID, &rv.ClientID, &rv.AuthorName, &rv.Device,
		&rv.Rating, &rv.Text, &rv.Status, &rv.CreatedAt)
	return rv, err
}

func (s *Server) handleReviews(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status == "" {
		status = model.ReviewApproved
	}
	u := s.optionalUser(r)
	if status != model.ReviewApproved {
		if u == nil || !model.HasPermission(u, model.PermManageRequests) {
			status = model.ReviewApproved
		}
	}
	page, limit, offset := pageParams(r)

	ctx, cancel := contextWithTimeout(r, 8*time.Second)
	defer cancel()

	where := `status = $1::review_status`
	args := []any{status}

	var total int
	if err := s.DB.QueryRow(ctx, `SELECT count(*) FROM reviews WHERE `+where, args...).Scan(&total); err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось посчитать отзывы")
		return
	}

	rows, err := s.DB.Query(ctx, `
		SELECT id, request_id, client_id, author_name, device, rating, text, status::text, created_at
		FROM reviews WHERE `+where+`
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`, append(args, limit, offset)...)
	if err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось загрузить отзывы")
		return
	}
	defer rows.Close()

	items := make([]model.Review, 0, limit)
	for rows.Next() {
		rv, err := scanReview(rows)
		if err != nil {
			logDBError(s.Logger, r, err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Ошибка чтения отзывов")
			return
		}
		items = append(items, rv)
	}

	var avg float64
	var approved int
	_ = s.DB.QueryRow(ctx, `
		SELECT COALESCE(round(avg(rating)::numeric, 1), 0)::float8, count(*)
		FROM reviews WHERE status = 'approved'`).Scan(&avg, &approved)

	writeJSON(w, http.StatusOK, map[string]any{
		"items": items, "total": total, "page": page, "limit": limit,
		"average": avg, "approved": approved,
	})
}

func (s *Server) handleMyReviews(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()

	rows, err := s.DB.Query(ctx, `
		SELECT id, request_id, client_id, author_name, device, rating, text, status::text, created_at
		FROM reviews WHERE client_id = $1 ORDER BY created_at DESC`, u.ID)
	if err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось загрузить отзывы")
		return
	}
	defer rows.Close()

	items := make([]model.Review, 0, 5)
	for rows.Next() {
		rv, err := scanReview(rows)
		if err != nil {
			logDBError(s.Logger, r, err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Ошибка чтения отзывов")
			return
		}
		items = append(items, rv)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

type reviewInput struct {
	RequestID  *int64 `json:"request_id"`
	AuthorName string `json:"author_name"`
	Device     string `json:"device"`
	Rating     int    `json:"rating"`
	Text       string `json:"text"`
}

func (s *Server) handleCreateReview(w http.ResponseWriter, r *http.Request) {
	var in reviewInput
	if !decodeJSON(w, r, &in) {
		return
	}
	u := currentUser(r)

	fields := map[string]string{}
	if in.Rating < 1 || in.Rating > 5 {
		fields["rating"] = "Поставьте оценку от 1 до 5"
	}
	text := cleanString(in.Text, 4000)
	if len([]rune(text)) < 10 {
		fields["text"] = "Отзыв должен быть не короче 10 символов"
	}
	author := cleanString(in.AuthorName, 120)
	if author == "" {
		author = u.FullName
	}
	if len(fields) > 0 {
		writeFieldError(w, fields)
		return
	}

	ctx, cancel := contextWithTimeout(r, 8*time.Second)
	defer cancel()

	device := cleanString(in.Device, 200)
	var requestID *int64
	if in.RequestID != nil {
		req, err := getRequest(ctx, s.DB, *in.RequestID)
		if err != nil || req.ClientID == nil || *req.ClientID != u.ID {
			writeFieldError(w, map[string]string{"request_id": "Заявка не найдена"})
			return
		}
		if req.Status != model.StatusPaid && req.Status != model.StatusCompleted {
			writeFieldError(w, map[string]string{"request_id": "Отзыв можно оставить после выполнения работ"})
			return
		}
		var exists bool
		if err := s.DB.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM reviews WHERE request_id = $1)`, req.ID).Scan(&exists); err != nil {
			logDBError(s.Logger, r, err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось проверить отзыв")
			return
		}
		if exists {
			writeError(w, http.StatusConflict, "review_exists", "Отзыв по этой заявке уже оставлен")
			return
		}
		requestID = &req.ID
		if device == "" {
			device = req.EquipmentType
			if req.BrandModel != "" {
				device += " " + req.BrandModel
			}
		}
	}
	if device == "" {
		device = "Ремонт бытовой техники"
	}

	var id int64
	err := s.DB.QueryRow(ctx, `
		INSERT INTO reviews (request_id, client_id, author_name, device, rating, text, status)
		VALUES ($1, $2, $3, $4, $5, $6, 'pending'::review_status)
		RETURNING id`,
		requestID, u.ID, author, device, in.Rating, text).Scan(&id)
	if err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось сохранить отзыв")
		return
	}

	var rv model.Review
	_ = s.DB.QueryRow(ctx, `
		SELECT id, request_id, client_id, author_name, device, rating, text, status::text, created_at
		FROM reviews WHERE id = $1`, id,
	).Scan(&rv.ID, &rv.RequestID, &rv.ClientID, &rv.AuthorName, &rv.Device, &rv.Rating, &rv.Text, &rv.Status, &rv.CreatedAt)

	writeJSON(w, http.StatusCreated, map[string]any{
		"review":  rv,
		"message": "Спасибо! Отзыв отправлен на модерацию.",
	})
}

func (s *Server) handleModerateReview(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "Некорректный номер отзыва")
		return
	}
	var in struct {
		Status string `json:"status"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.Status != model.ReviewApproved && in.Status != model.ReviewRejected {
		writeFieldError(w, map[string]string{"status": "Допустимо: approved или rejected"})
		return
	}
	u := currentUser(r)

	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()

	var rv model.Review
	err := s.DB.QueryRow(ctx, `
		UPDATE reviews SET status = $1::review_status, moderated_by = $2
		WHERE id = $3
		RETURNING id, request_id, client_id, author_name, device, rating, text, status::text, created_at`,
		in.Status, u.ID, id,
	).Scan(&rv.ID, &rv.RequestID, &rv.ClientID, &rv.AuthorName, &rv.Device, &rv.Rating, &rv.Text, &rv.Status, &rv.CreatedAt)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "not_found", "Отзыв не найден")
			return
		}
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось обновить отзыв")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"review": rv})
}
