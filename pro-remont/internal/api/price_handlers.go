package api

import (
	"net/http"
	"time"

	"pro-remont/internal/model"
)

func (s *Server) handlePriceList(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 8*time.Second)
	defer cancel()

	rows, err := s.DB.Query(ctx, `
		SELECT s.id, s.category_id, c.slug, c.title, s.title, s.description,
		       s.price::float8, s.warranty_months, s.is_active
		FROM services s
		JOIN categories c ON c.id = s.category_id
		ORDER BY c.sort_order, s.sort_order, s.id`)
	if err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось загрузить прайс-лист")
		return
	}
	defer rows.Close()

	items := make([]model.Service, 0, 32)
	for rows.Next() {
		var item model.Service
		if err := rows.Scan(&item.ID, &item.CategoryID, &item.CategorySlug, &item.CategoryTitle,
			&item.Title, &item.Description, &item.Price, &item.WarrantyMonths, &item.IsActive); err != nil {
			logDBError(s.Logger, r, err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Ошибка чтения прайс-листа")
			return
		}
		items = append(items, item)
	}

	catRows, err := s.DB.Query(ctx, `SELECT id, slug, title, description, base_price::float8 FROM categories ORDER BY sort_order, id`)
	if err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось загрузить категории")
		return
	}
	defer catRows.Close()
	categories := make([]model.Category, 0, 8)
	for catRows.Next() {
		var c model.Category
		if err := catRows.Scan(&c.ID, &c.Slug, &c.Title, &c.Description, &c.BasePrice); err != nil {
			logDBError(s.Logger, r, err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Ошибка чтения категорий")
			return
		}
		categories = append(categories, c)
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": items, "categories": categories})
}

type priceInput struct {
	Price          *float64 `json:"price"`
	WarrantyMonths *int     `json:"warranty_months"`
	IsActive       *bool    `json:"is_active"`
	Title          *string  `json:"title"`
	Description    *string  `json:"description"`
}

func (s *Server) handleUpdatePrice(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "Некорректный идентификатор услуги")
		return
	}
	var in priceInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.Price == nil && in.WarrantyMonths == nil && in.IsActive == nil && in.Title == nil && in.Description == nil {
		writeError(w, http.StatusBadRequest, "bad_request", "Нет данных для обновления")
		return
	}
	if in.Price != nil && (*in.Price < 0 || *in.Price > 1_000_000) {
		writeFieldError(w, map[string]string{"price": "Стоимость должна быть числом от 0 до 1 000 000"})
		return
	}
	if in.WarrantyMonths != nil && (*in.WarrantyMonths < 0 || *in.WarrantyMonths > 120) {
		writeFieldError(w, map[string]string{"warranty_months": "Гарантия должна быть от 0 до 120 месяцев"})
		return
	}

	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()

	item, err := scanService(s.DB.QueryRow(ctx, `
		UPDATE services SET
			price = COALESCE($1::numeric, price),
			warranty_months = COALESCE($2::int, warranty_months),
			is_active = COALESCE($3::boolean, is_active),
			title = COALESCE($4::text, title),
			description = COALESCE($5::text, description)
		WHERE id = $6
		RETURNING id, category_id,
			(SELECT slug FROM categories c WHERE c.id = services.category_id),
			(SELECT title FROM categories c WHERE c.id = services.category_id),
			title, description, price::float8, warranty_months, is_active`,
		in.Price, in.WarrantyMonths, in.IsActive, in.Title, in.Description, id))
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "not_found", "Услуга не найдена")
			return
		}
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось обновить цену")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"service": item,
		"message": "Прайс-лист обновлён. Изменения сразу видны на сайте.",
	})
}

func scanService(sc rowScanner) (model.Service, error) {
	var item model.Service
	err := sc.Scan(&item.ID, &item.CategoryID, &item.CategorySlug, &item.CategoryTitle,
		&item.Title, &item.Description, &item.Price, &item.WarrantyMonths, &item.IsActive)
	return item, err
}
