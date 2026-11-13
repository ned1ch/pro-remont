package api

import (
	"net/http"
	"time"

	"pro-remont/internal/model"
)

func (s *Server) handlePersonalOffers(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()

	var orders int
	if err := s.DB.QueryRow(ctx, `
		SELECT count(*) FROM requests
		WHERE client_id = $1 AND status NOT IN ('new', 'cancelled')`, u.ID).Scan(&orders); err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось рассчитать предложения")
		return
	}

	rows, err := s.DB.Query(ctx, `
		SELECT code, title, description, discount_percent, audience, min_orders
		FROM offers WHERE is_active ORDER BY discount_percent DESC, id`)
	if err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось загрузить предложения")
		return
	}
	defer rows.Close()

	items := make([]model.Offer, 0, 4)
	for rows.Next() {
		var o model.Offer
		var audience string
		var minOrders int
		if err := rows.Scan(&o.Code, &o.Title, &o.Description, &o.DiscountPercent,
			&audience, &minOrders); err != nil {
			logDBError(s.Logger, r, err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Ошибка чтения предложений")
			return
		}
		switch audience {
		case "new":
			if orders > 0 {
				continue
			}
			o.Reason = "Приветственное предложение для новых клиентов"
		case "loyal":
			if orders < minOrders {
				continue
			}
			o.Reason = "Персональная скидка постоянного клиента"
		default:
			o.Reason = "Действует для всех клиентов сервиса"
		}
		items = append(items, o)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "orders": orders})
}

type statCard struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Hint  string `json:"hint"`
	Tone  string `json:"tone"`
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	ctx, cancel := contextWithTimeout(r, 8*time.Second)
	defer cancel()

	statusCounts := map[string]int{}
	statusQuery := `SELECT status::text, count(*) FROM requests`
	statusArgs := []any{}
	switch {
	case model.IsStaff(u.Role):
		// Сводка по всем заявкам доступна только сотрудникам.
	case u.Role == model.RoleMaster:
		statusQuery += ` WHERE master_id = $1`
		statusArgs = append(statusArgs, u.ID)
	default:
		statusQuery += ` WHERE client_id = $1`
		statusArgs = append(statusArgs, u.ID)
	}
	statusQuery += ` GROUP BY status`
	rows, err := s.DB.Query(ctx, statusQuery, statusArgs...)
	if err != nil {
		logDBError(s.Logger, r, err)
	} else {
		defer rows.Close()
		for rows.Next() {
			var st string
			var n int
			if err := rows.Scan(&st, &n); err == nil {
				statusCounts[st] = n
			}
		}
	}

	cards := make([]statCard, 0, 5)

	switch u.Role {
	case model.RoleClient:
		var total, active, paidCount int
		var spent float64
		_ = s.DB.QueryRow(ctx, `SELECT count(*) FROM requests WHERE client_id = $1`, u.ID).Scan(&total)
		_ = s.DB.QueryRow(ctx, `
			SELECT count(*) FROM requests
			WHERE client_id = $1 AND status NOT IN ('paid', 'cancelled')`, u.ID).Scan(&active)
		_ = s.DB.QueryRow(ctx, `
			SELECT count(*) FROM requests WHERE client_id = $1 AND status = 'paid'`, u.ID).Scan(&paidCount)
		_ = s.DB.QueryRow(ctx, `
			SELECT COALESCE(sum(i.amount), 0)::float8 FROM invoices i
			JOIN requests r ON r.id = i.request_id
			WHERE r.client_id = $1 AND i.status = 'paid'`, u.ID).Scan(&spent)
		cards = append(cards,
			statCard{"Всего заявок", itoa(total), "За всё время", "default"},
			statCard{"Активные ремонты", itoa(active), "В работе и ожидают мастера", "accent"},
			statCard{"Оплачено", money(spent), "По закрытым счетам", "success"},
			statCard{"Завершено", itoa(paidCount), "Можно оставить отзыв", "default"},
		)

	case model.RoleMaster:
		var assigned, ready, doneMonth int
		var rating float64
		_ = s.DB.QueryRow(ctx, `
			SELECT count(*) FROM requests
			WHERE master_id = $1 AND status IN ('assigned', 'in_progress')`, u.ID).Scan(&assigned)
		_ = s.DB.QueryRow(ctx, `
			SELECT count(*) FROM requests WHERE master_id = $1 AND status = 'ready'`, u.ID).Scan(&ready)
		_ = s.DB.QueryRow(ctx, `
			SELECT count(*) FROM requests
			WHERE master_id = $1 AND status IN ('completed', 'paid')
			  AND COALESCE(completed_at, updated_at) >= date_trunc('month', now())`, u.ID).Scan(&doneMonth)
		_ = s.DB.QueryRow(ctx, `
			SELECT COALESCE(rating, 0)::float8 FROM master_profiles WHERE user_id = $1`, u.ID).Scan(&rating)
		cards = append(cards,
			statCard{"В работе", itoa(assigned), "Назначенные заявки", "accent"},
			statCard{"Готово к выдаче", itoa(ready), "Ожидают закрытия", "success"},
			statCard{"Выполнено за месяц", itoa(doneMonth), "Завершённые ремонты", "default"},
			statCard{"Рейтинг", formatRating(rating), "Средняя оценка клиентов", "warning"},
		)

	default:
		var newCount, active, ready, users int
		var revenue float64
		var avgRating float64
		_ = s.DB.QueryRow(ctx, `SELECT count(*) FROM requests WHERE status = 'new'`).Scan(&newCount)
		_ = s.DB.QueryRow(ctx, `
			SELECT count(*) FROM requests WHERE status IN ('accepted', 'assigned', 'in_progress')`).Scan(&active)
		_ = s.DB.QueryRow(ctx, `
			SELECT count(*) FROM requests WHERE status IN ('ready', 'completed')`).Scan(&ready)
		_ = s.DB.QueryRow(ctx, `SELECT count(*) FROM users WHERE is_active`).Scan(&users)
		_ = s.DB.QueryRow(ctx, `
			SELECT COALESCE(sum(amount), 0)::float8 FROM invoices WHERE status = 'paid'`).Scan(&revenue)
		_ = s.DB.QueryRow(ctx, `
			SELECT COALESCE(round(avg(rating)::numeric, 1), 0)::float8 FROM reviews WHERE status = 'approved'`).Scan(&avgRating)
		cards = append(cards,
			statCard{"Новые заявки", itoa(newCount), "Требуют обработки", "warning"},
			statCard{"В работе", itoa(active), "Приняты и назначены", "accent"},
			statCard{"Готово / выполнено", itoa(ready), "Ожидают оплаты", "success"},
			statCard{"Выручка", money(revenue), "По оплаченным счетам", "success"},
			statCard{"Пользователи", itoa(users), "Активные аккаунты", "default"},
			statCard{"Средняя оценка", formatRating(avgRating), "Одобренные отзывы", "warning"},
		)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"role": u.Role, "cards": cards, "status_counts": statusCounts,
	})
}

func money(v float64) string {
	return formatRub(v)
}

func formatRating(v float64) string {
	return itoa(int(v)) + "." + itoa(int(v*10)%10)
}
