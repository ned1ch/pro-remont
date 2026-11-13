package api

import (
	"net/http"
	"strings"
	"time"

	"pro-remont/internal/model"
)

func (s *Server) handleCategories(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()

	rows, err := s.DB.Query(ctx, `
		SELECT c.id, c.slug, c.title, c.description, c.base_price::float8,
		       COUNT(s.id) FILTER (WHERE s.is_active) AS services
		FROM categories c
		LEFT JOIN services s ON s.category_id = c.id
		GROUP BY c.id
		ORDER BY c.sort_order, c.id`)
	if err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось загрузить категории")
		return
	}
	defer rows.Close()

	items := make([]model.Category, 0, 8)
	for rows.Next() {
		var c model.Category
		if err := rows.Scan(&c.ID, &c.Slug, &c.Title, &c.Description, &c.BasePrice, &c.Services); err != nil {
			logDBError(s.Logger, r, err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Ошибка чтения категорий")
			return
		}
		items = append(items, c)
	}
	writeJSON(w, http.StatusOK, map[string]any{"categories": items})
}

func (s *Server) handleCatalogItems(w http.ResponseWriter, r *http.Request) {
	q := cleanString(r.URL.Query().Get("q"), 100)
	category := cleanString(r.URL.Query().Get("category"), 50)

	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()

	rows, err := s.DB.Query(ctx, `
		SELECT s.id, s.category_id, c.slug, c.title, s.title, s.description,
		       s.price::float8, s.warranty_months, s.is_active
		FROM services s
		JOIN categories c ON c.id = s.category_id
		WHERE s.is_active
		  AND ($1 = '' OR c.slug = $1)
		  AND ($2 = '' OR s.title ILIKE '%' || $2 || '%' OR s.description ILIKE '%' || $2 || '%')
		ORDER BY c.sort_order, s.sort_order, s.id`, category, q)
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
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func (s *Server) handleMasters(w http.ResponseWriter, r *http.Request) {
	onlyAvailable := r.URL.Query().Get("available") == "true"
	includeAll := r.URL.Query().Get("all") == "true"

	var viewer *model.User
	if includeAll {
		viewer = s.optionalUser(r)
		if viewer == nil || !(model.IsStaff(viewer.Role) || model.HasPermission(viewer, model.PermAssignMasters)) {
			writeError(w, http.StatusForbidden, "forbidden", "Список всех сотрудников доступен только менеджеру")
			return
		}
	}

	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()

	rows, err := s.DB.Query(ctx, `
		SELECT u.id, u.full_name, COALESCE(u.email, ''), COALESCE(u.phone, ''),
		       mp.specialization, mp.bio, mp.experience_years, mp.rating::float8,
		       (mp.is_available AND COALESCE(a.cnt, 0) = 0) AS is_available,
		       COALESCE(a.cnt, 0), COALESCE(d.cnt, 0)
		FROM users u
		JOIN master_profiles mp ON mp.user_id = u.id
		LEFT JOIN LATERAL (
			SELECT count(*) AS cnt FROM requests
			WHERE master_id = u.id AND status IN ('assigned', 'in_progress')
		) a ON TRUE
		LEFT JOIN LATERAL (
			SELECT count(*) AS cnt FROM requests
			WHERE master_id = u.id AND status IN ('completed', 'paid')
		) d ON TRUE
		WHERE u.role = 'master' AND u.is_active
		ORDER BY is_available DESC, mp.rating DESC, u.full_name`)
	if err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось загрузить мастеров")
		return
	}
	defer rows.Close()

	items := make([]model.Master, 0, 8)
	for rows.Next() {
		var m model.Master
		if err := rows.Scan(&m.ID, &m.FullName, &m.Email, &m.Phone, &m.Specialization, &m.Bio,
			&m.ExperienceYears, &m.Rating, &m.IsAvailable, &m.ActiveRequests, &m.CompletedRequests); err != nil {
			logDBError(s.Logger, r, err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Ошибка чтения мастеров")
			return
		}
		if onlyAvailable && !m.IsAvailable {
			continue
		}
		if !includeAll {
			m.Email = ""
			m.Phone = ""
		}
		items = append(items, m)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

// sanitizeLike экранирует % и _ для ILIKE, если понадобится поиск по шаблону.
func sanitizeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "%", `\%`)
	s = strings.ReplaceAll(s, "_", `\_`)
	return s
}
