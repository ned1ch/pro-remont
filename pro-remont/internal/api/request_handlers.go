package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"pro-remont/internal/auth"
	"pro-remont/internal/model"
)

const requestSelect = `
	SELECT r.id, r.public_number, r.client_id, r.customer_name, r.phone, COALESCE(r.email, ''),
	       r.address, r.category_id, COALESCE(c.slug, ''), COALESCE(c.title, ''),
	       r.service_id, COALESCE(s.title, ''), r.equipment_type, r.brand_model, r.description,
	       r.status::text, r.master_id, COALESCE(m.full_name, ''), r.total_amount::float8,
	       r.manager_comment, r.master_comment, r.accepted_at, r.completed_at, r.created_at, r.updated_at
	FROM requests r
	LEFT JOIN categories c ON c.id = r.category_id
	LEFT JOIN services s ON s.id = r.service_id
	LEFT JOIN users m ON m.id = r.master_id`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanRequest(sc rowScanner) (model.Request, error) {
	var req model.Request
	err := sc.Scan(
		&req.ID, &req.PublicNumber, &req.ClientID, &req.CustomerName, &req.Phone, &req.Email,
		&req.Address, &req.CategoryID, &req.CategorySlug, &req.CategoryTitle,
		&req.ServiceID, &req.ServiceTitle, &req.EquipmentType, &req.BrandModel, &req.Description,
		&req.Status, &req.MasterID, &req.MasterName, &req.TotalAmount,
		&req.ManagerComment, &req.MasterComment, &req.AcceptedAt, &req.CompletedAt,
		&req.CreatedAt, &req.UpdatedAt,
	)
	return req, err
}

type requestQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func getRequest(ctx context.Context, q requestQuerier, id int64) (*model.Request, error) {
	req, err := scanRequest(q.QueryRow(ctx, requestSelect+` WHERE r.id = $1`, id))
	if err != nil {
		return nil, err
	}
	return &req, nil
}

func getRequestForUpdate(ctx context.Context, tx pgx.Tx, id int64) (*model.Request, error) {
	req, err := scanRequest(tx.QueryRow(ctx, requestSelect+` WHERE r.id = $1 FOR UPDATE OF r`, id))
	if err != nil {
		return nil, err
	}
	return &req, nil
}

func canViewRequest(u *model.User, req *model.Request) bool {
	if u.Role == model.RoleAdmin || model.HasPermission(u, model.PermManageRequests) {
		return true
	}
	if req.ClientID != nil && *req.ClientID == u.ID {
		return true
	}
	if req.MasterID != nil && *req.MasterID == u.ID {
		return true
	}
	return false
}

type createRequestInput struct {
	CustomerName  string `json:"customer_name"`
	Phone         string `json:"phone"`
	Email         string `json:"email"`
	Address       string `json:"address"`
	CategoryID    *int64 `json:"category_id"`
	ServiceID     *int64 `json:"service_id"`
	EquipmentType string `json:"equipment_type"`
	BrandModel    string `json:"brand_model"`
	Description   string `json:"description"`
}

func (s *Server) handleCreateRequest(w http.ResponseWriter, r *http.Request) {
	var in createRequestInput
	if !decodeJSON(w, r, &in) {
		return
	}

	viewer := s.optionalUser(r)

	name := cleanString(in.CustomerName, 120)
	address := cleanString(in.Address, 255)
	var email string
	if strings.TrimSpace(in.Email) != "" {
		if e, valid := auth.NormalizeEmail(in.Email); valid {
			email = e
		}
	}
	phone, phoneOK := auth.NormalizePhone(in.Phone)

	// Авторизованному клиенту подставляем данные профиля в пустые поля.
	if viewer != nil {
		if name == "" {
			name = viewer.FullName
		}
		if !phoneOK && viewer.Phone != "" {
			phone, phoneOK = viewer.Phone, true
		}
		if email == "" {
			email = viewer.Email
		}
		if address == "" {
			address = viewer.Address
		}
	}

	fields := map[string]string{}
	if len([]rune(name)) < 2 {
		fields["customer_name"] = "Укажите имя"
	}
	if !phoneOK {
		fields["phone"] = "Введите корректный телефон"
	}
	if len([]rune(address)) < 5 {
		fields["address"] = "Укажите адрес выезда мастера"
	}
	description := cleanString(in.Description, 4000)
	if len([]rune(description)) < 5 {
		fields["description"] = "Опишите неисправность (минимум 5 символов)"
	}
	if in.CategoryID == nil && in.ServiceID == nil {
		fields["category_id"] = "Выберите тип техники"
	}
	if strings.TrimSpace(in.Email) != "" && email == "" {
		fields["email"] = "Некорректный email"
	}
	if len(fields) > 0 {
		writeFieldError(w, fields)
		return
	}

	ctx, cancel := contextWithTimeout(r, 8*time.Second)
	defer cancel()

	var categoryID int64
	var categoryTitle, categorySlug string
	var basePrice float64
	var serviceID *int64
	var serviceTitle string
	var amount float64

	if in.ServiceID != nil {
		err := s.DB.QueryRow(ctx, `
			SELECT s.id, s.title, s.price::float8, s.category_id, c.title, c.slug
			FROM services s JOIN categories c ON c.id = s.category_id
			WHERE s.id = $1 AND s.is_active`, *in.ServiceID,
		).Scan(&serviceID, &serviceTitle, &amount, &categoryID, &categoryTitle, &categorySlug)
		if err != nil {
			if isNotFound(err) {
				writeFieldError(w, map[string]string{"service_id": "Услуга не найдена"})
				return
			}
			logDBError(s.Logger, r, err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось получить услугу")
			return
		}
	} else {
		err := s.DB.QueryRow(ctx, `
			SELECT id, title, slug, base_price::float8 FROM categories WHERE id = $1`, *in.CategoryID,
		).Scan(&categoryID, &categoryTitle, &categorySlug, &basePrice)
		if err != nil {
			if isNotFound(err) {
				writeFieldError(w, map[string]string{"category_id": "Категория не найдена"})
				return
			}
			logDBError(s.Logger, r, err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось получить категорию")
			return
		}
		amount = basePrice
	}

	equipment := cleanString(in.EquipmentType, 120)
	if equipment == "" {
		equipment = categoryTitle
	}
	brand := cleanString(in.BrandModel, 120)

	tx, err := s.DB.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось создать заявку")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var clientID any
	if viewer != nil {
		clientID = viewer.ID
	}
	var emailArg any
	if email != "" {
		emailArg = email
	}

	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO requests (
			client_id, customer_name, phone, email, address, category_id, service_id,
			equipment_type, brand_model, description, total_amount
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id`,
		clientID, name, phone, emailArg, address, categoryID, serviceID,
		equipment, brand, description, amount).Scan(&id)
	if err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось сохранить заявку")
		return
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO request_history (request_id, from_status, to_status, changed_by, comment)
		VALUES ($1, NULL, 'new'::request_status, $2, 'Заявка создана через сайт')`,
		id, clientID); err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось сохранить историю заявки")
		return
	}

	if viewer != nil {
		_, _ = tx.Exec(ctx, `
			INSERT INTO notifications (user_id, request_id, type, title, message)
			VALUES ($1, $2, 'request_created', 'Заявка принята', $3)`,
			viewer.ID, id, "Мы получили вашу заявку и свяжемся с вами для уточнения деталей.")
	}
	_, _ = tx.Exec(ctx, `
		INSERT INTO notifications (user_id, request_id, type, title, message)
		SELECT u.id, $1, 'request_created', 'Новая заявка на ремонт', $2
		FROM users u WHERE u.role IN ('manager', 'admin') AND u.is_active`,
		id, fmt.Sprintf("Поступила новая заявка от %s (%s).", name, phone))

	if err := tx.Commit(ctx); err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось сохранить заявку")
		return
	}

	req, err := getRequest(ctx, s.DB, id)
	if err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Заявка создана, но не удалось загрузить данные")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"request": req,
		"message": "Заявка создана. Номер: " + req.PublicNumber,
	})
}

func (s *Server) handleMyRequests(w http.ResponseWriter, r *http.Request) {
	s.listClientRequests(w, r, false)
}

func (s *Server) handleMyHistory(w http.ResponseWriter, r *http.Request) {
	s.listClientRequests(w, r, true)
}

func (s *Server) listClientRequests(w http.ResponseWriter, r *http.Request, history bool) {
	u := currentUser(r)
	ctx, cancel := contextWithTimeout(r, 8*time.Second)
	defer cancel()

	condition := `r.status NOT IN ('paid', 'cancelled')`
	if history {
		condition = `r.status IN ('paid', 'cancelled')`
	}
	rows, err := s.DB.Query(ctx, requestSelect+`
		WHERE r.client_id = $1 AND `+condition+`
		ORDER BY r.created_at DESC`, u.ID)
	if err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось загрузить заявки")
		return
	}
	defer rows.Close()

	items := make([]model.Request, 0, 8)
	for rows.Next() {
		req, err := scanRequest(rows)
		if err != nil {
			logDBError(s.Logger, r, err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Ошибка чтения заявок")
			return
		}
		items = append(items, req)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func (s *Server) handleGetRequest(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "Некорректный номер заявки")
		return
	}
	u := currentUser(r)

	ctx, cancel := contextWithTimeout(r, 8*time.Second)
	defer cancel()

	req, err := getRequest(ctx, s.DB, id)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "not_found", "Заявка не найдена")
			return
		}
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось загрузить заявку")
		return
	}
	if !canViewRequest(u, req) {
		writeError(w, http.StatusForbidden, "forbidden", "Нет доступа к этой заявке")
		return
	}

	history, err := s.loadHistory(ctx, id)
	if err != nil {
		logDBError(s.Logger, r, err)
	}
	writeJSON(w, http.StatusOK, map[string]any{"request": req, "history": history})
}

func (s *Server) loadHistory(ctx context.Context, requestID int64) ([]model.HistoryEntry, error) {
	rows, err := s.DB.Query(ctx, `
		SELECT h.id, COALESCE(h.from_status::text, ''), h.to_status::text, h.comment,
		       COALESCE(u.full_name, 'Система'), h.created_at
		FROM request_history h
		LEFT JOIN users u ON u.id = h.changed_by
		WHERE h.request_id = $1
		ORDER BY h.created_at, h.id`, requestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.HistoryEntry, 0, 8)
	for rows.Next() {
		var e model.HistoryEntry
		if err := rows.Scan(&e.ID, &e.FromStatus, &e.ToStatus, &e.Comment, &e.ChangedBy, &e.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, e)
	}
	return items, rows.Err()
}

func (s *Server) handleRequestHistory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "Некорректный номер заявки")
		return
	}
	u := currentUser(r)
	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()

	req, err := getRequest(ctx, s.DB, id)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "not_found", "Заявка не найдена")
			return
		}
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось загрузить заявку")
		return
	}
	if !canViewRequest(u, req) {
		writeError(w, http.StatusForbidden, "forbidden", "Нет доступа к этой заявке")
		return
	}
	items, err := s.loadHistory(ctx, id)
	if err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось загрузить историю")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleAcceptRequest(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "Некорректный номер заявки")
		return
	}
	u := currentUser(r)

	var in struct {
		Comment string `json:"comment"`
	}
	if r.ContentLength > 0 && !decodeJSON(w, r, &in) {
		return
	}

	ctx, cancel := contextWithTimeout(r, 8*time.Second)
	defer cancel()

	tx, err := s.DB.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось обработать заявку")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	req, err := getRequestForUpdate(ctx, tx, id)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "not_found", "Заявка не найдена")
			return
		}
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось загрузить заявку")
		return
	}
	if req.Status != model.StatusNew {
		writeError(w, http.StatusConflict, "already_taken", "Заявка уже обрабатывается другим сотрудником")
		return
	}

	comment := cleanString(in.Comment, 500)
	if comment == "" {
		comment = "Менеджер принял заявку в обработку"
	}
	if _, err := tx.Exec(ctx, `
		UPDATE requests SET status = 'accepted', accepted_at = now() WHERE id = $1`, id); err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось принять заявку")
		return
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO request_history (request_id, from_status, to_status, changed_by, comment)
		VALUES ($1, 'new', 'accepted', $2, $3)`, id, u.ID, comment); err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось сохранить историю")
		return
	}
	if req.ClientID != nil {
		_, _ = tx.Exec(ctx, `
			INSERT INTO notifications (user_id, request_id, type, title, message)
			VALUES ($1, $2, 'request_accepted', 'Заявка принята в работу', $3)`,
			*req.ClientID, id, "Менеджер принял вашу заявку. Мы подберём мастера и сообщим подробности.")
	}
	if err := tx.Commit(ctx); err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось принять заявку")
		return
	}

	updated, _ := getRequest(ctx, s.DB, id)
	writeJSON(w, http.StatusOK, map[string]any{"request": updated})
}

type assignInput struct {
	MasterID int64  `json:"master_id"`
	Comment  string `json:"comment"`
	Force    bool   `json:"force"`
}

func (s *Server) handleAssignMaster(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "Некорректный номер заявки")
		return
	}
	var in assignInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.MasterID <= 0 {
		writeFieldError(w, map[string]string{"master_id": "Выберите мастера"})
		return
	}
	u := currentUser(r)

	ctx, cancel := contextWithTimeout(r, 8*time.Second)
	defer cancel()

	tx, err := s.DB.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось назначить мастера")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	req, err := getRequestForUpdate(ctx, tx, id)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "not_found", "Заявка не найдена")
			return
		}
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось загрузить заявку")
		return
	}
	if req.Status != model.StatusAccepted {
		writeError(w, http.StatusConflict, "invalid_status", "Назначить мастера можно только для принятой заявки")
		return
	}

	var masterName, specialization string
	var profileAvailable bool
	var activeCount int
	err = tx.QueryRow(ctx, `
		SELECT u.full_name, mp.specialization, mp.is_available,
		       (SELECT count(*) FROM requests ar WHERE ar.master_id = u.id AND ar.status IN ('assigned', 'in_progress'))
		FROM users u JOIN master_profiles mp ON mp.user_id = u.id
		WHERE u.id = $1 AND u.role = 'master' AND u.is_active`, in.MasterID,
	).Scan(&masterName, &specialization, &profileAvailable, &activeCount)
	if err != nil {
		if isNotFound(err) {
			writeFieldError(w, map[string]string{"master_id": "Мастер не найден или недоступен"})
			return
		}
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось проверить мастера")
		return
	}
	busy := !profileAvailable || activeCount > 0
	if busy && !in.Force {
		writeJSON(w, http.StatusConflict, errorBody{Error: errorInfo{
			Code:    "master_busy",
			Message: "Мастер занят другой заявкой. Подтвердите назначение принудительно.",
			Fields:  map[string]string{"master_id": masterName + " — занят (" + specialization + ")"},
		}})
		return
	}

	comment := cleanString(in.Comment, 500)
	if comment == "" {
		comment = "Назначен мастер: " + masterName
	}
	if _, err := tx.Exec(ctx, `
		UPDATE requests SET status = 'assigned', master_id = $1 WHERE id = $2`, in.MasterID, id); err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось назначить мастера")
		return
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO request_history (request_id, from_status, to_status, changed_by, comment)
		VALUES ($1, 'accepted', 'assigned', $2, $3)`, id, u.ID, comment); err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось сохранить историю")
		return
	}
	_, _ = tx.Exec(ctx, `
		INSERT INTO notifications (user_id, request_id, type, title, message)
		VALUES ($1, $2, 'master_assigned', 'Вам назначена заявка', $3)`,
		in.MasterID, id, fmt.Sprintf("%s: %s. Адрес и контакты — в личном кабинете.", req.EquipmentType, req.Description))
	if req.ClientID != nil {
		_, _ = tx.Exec(ctx, `
			INSERT INTO notifications (user_id, request_id, type, title, message)
			VALUES ($1, $2, 'master_assigned', 'Мастер назначен', $3)`,
			*req.ClientID, id, fmt.Sprintf("За вашей заявкой закреплён мастер %s (%s).", masterName, specialization))
	}
	if err := tx.Commit(ctx); err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось назначить мастера")
		return
	}

	updated, _ := getRequest(ctx, s.DB, id)
	writeJSON(w, http.StatusOK, map[string]any{"request": updated})
}

type statusInput struct {
	Status  string `json:"status"`
	Comment string `json:"comment"`
}

func (s *Server) handleChangeStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "Некорректный номер заявки")
		return
	}
	var in statusInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if _, exists := model.StatusLabels[in.Status]; !exists || in.Status == model.StatusNew {
		writeFieldError(w, map[string]string{"status": "Некорректный статус"})
		return
	}
	u := currentUser(r)

	ctx, cancel := contextWithTimeout(r, 8*time.Second)
	defer cancel()

	tx, err := s.DB.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось изменить статус")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	req, err := getRequestForUpdate(ctx, tx, id)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "not_found", "Заявка не найдена")
			return
		}
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось загрузить заявку")
		return
	}

	switch {
	case u.Role == model.RoleMaster:
		if req.MasterID == nil || *req.MasterID != u.ID {
			writeError(w, http.StatusForbidden, "forbidden", "Заявка не назначена на вас")
			return
		}
		if !model.CanRoleSetStatus(model.RoleMaster, req.Status, in.Status) {
			writeError(w, http.StatusConflict, "invalid_transition", "Недопустимый переход статуса для мастера")
			return
		}
	case u.Role == model.RoleClient:
		writeError(w, http.StatusForbidden, "forbidden", "Отменить заявку можно через отдельное действие")
		return
	default:
		if !model.HasPermission(u, model.PermManageRequests) {
			writeError(w, http.StatusForbidden, "forbidden", "Недостаточно прав")
			return
		}
		if !model.CanRoleSetStatus(u.Role, req.Status, in.Status) {
			writeError(w, http.StatusConflict, "invalid_transition", "Недопустимый переход статуса: "+
				model.StatusLabels[req.Status]+" → "+model.StatusLabels[in.Status])
			return
		}
	}

	comment := cleanString(in.Comment, 1000)
	if comment == "" {
		comment = "Статус изменён: " + model.StatusLabels[in.Status]
	}

	var completedAt any
	if in.Status == model.StatusCompleted {
		completedAt = time.Now()
	}
	_, err = tx.Exec(ctx, `
		UPDATE requests
		SET status = $1::request_status,
		    completed_at = CASE WHEN $1 = 'completed' THEN $2::timestamptz ELSE completed_at END,
		    accepted_at = CASE WHEN $1 = 'accepted' AND accepted_at IS NULL THEN now() ELSE accepted_at END
		WHERE id = $3`, in.Status, completedAt, id)
	if err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось изменить статус")
		return
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO request_history (request_id, from_status, to_status, changed_by, comment)
		VALUES ($1, $2::request_status, $3::request_status, $4, $5)`,
		id, req.Status, in.Status, u.ID, comment); err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось сохранить историю")
		return
	}

	// Если клиент оплатил заявку авансом, при завершении работ сразу
	// переводим её в статус «Оплачена», чтобы не оставлять закрытую заявку
	// с уже оплаченным счётом в статусе «Выполнена».
	finalStatus := in.Status
	if in.Status == model.StatusCompleted {
		var invoiceStatus string
		if err := tx.QueryRow(ctx,
			`SELECT status::text FROM invoices WHERE request_id = $1`, id).Scan(&invoiceStatus); err == nil &&
			invoiceStatus == model.InvoicePaid {
			if _, err := tx.Exec(ctx, `UPDATE requests SET status = 'paid' WHERE id = $1`, id); err != nil {
				logDBError(s.Logger, r, err)
				writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось обновить статус оплаты")
				return
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO request_history (request_id, from_status, to_status, changed_by, comment)
				VALUES ($1, 'completed', 'paid', $2, 'Счёт был оплачен ранее')`, id, u.ID); err != nil {
				logDBError(s.Logger, r, err)
			}
			finalStatus = model.StatusPaid
		}
	}

	if req.ClientID != nil {
		switch finalStatus {
		case model.StatusReady:
			_, _ = tx.Exec(ctx, `
				INSERT INTO notifications (user_id, request_id, type, title, message)
				VALUES ($1, $2, 'ready', 'Ремонт готов к выдаче', $3)`,
				*req.ClientID, id, "Ваша техника отремонтирована. Проверьте детали заявки и оплатите счёт.")
		case model.StatusPaid:
			_, _ = tx.Exec(ctx, `
				INSERT INTO notifications (user_id, request_id, type, title, message)
				VALUES ($1, $2, 'payment', 'Заявка оплачена', 'Платёж получен. Спасибо, что выбрали наш сервис!')`,
				*req.ClientID, id)
		case model.StatusCompleted:
			_, _ = tx.Exec(ctx, `
				INSERT INTO notifications (user_id, request_id, type, title, message)
				VALUES ($1, $2, 'status_changed', 'Работы выполнены', 'Ремонт завершён. Осталось оплатить заявку.')`,
				*req.ClientID, id)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось изменить статус")
		return
	}

	updated, _ := getRequest(ctx, s.DB, id)
	writeJSON(w, http.StatusOK, map[string]any{"request": updated})
}

func (s *Server) handleCancelRequest(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "Некорректный номер заявки")
		return
	}
	var in struct {
		Comment string `json:"comment"`
	}
	if r.ContentLength > 0 && !decodeJSON(w, r, &in) {
		return
	}
	u := currentUser(r)

	ctx, cancel := contextWithTimeout(r, 8*time.Second)
	defer cancel()

	tx, err := s.DB.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось отменить заявку")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	req, err := getRequestForUpdate(ctx, tx, id)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "not_found", "Заявка не найдена")
			return
		}
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось загрузить заявку")
		return
	}

	isOwner := req.ClientID != nil && *req.ClientID == u.ID
	if !isOwner && !model.HasPermission(u, model.PermManageRequests) {
		writeError(w, http.StatusForbidden, "forbidden", "Нет доступа к этой заявке")
		return
	}
	if isOwner && !model.HasPermission(u, model.PermManageRequests) {
		if req.Status != model.StatusNew && req.Status != model.StatusAccepted {
			writeError(w, http.StatusConflict, "invalid_status", "Отменить можно только новую или принятую заявку")
			return
		}
	}
	if req.Status == model.StatusPaid || req.Status == model.StatusCancelled {
		writeError(w, http.StatusConflict, "invalid_status", "Заявка уже закрыта")
		return
	}

	comment := cleanString(in.Comment, 500)
	if comment == "" {
		comment = "Заявка отменена"
	}
	if _, err := tx.Exec(ctx, `UPDATE requests SET status = 'cancelled' WHERE id = $1`, id); err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось отменить заявку")
		return
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO request_history (request_id, from_status, to_status, changed_by, comment)
		VALUES ($1, $2::request_status, 'cancelled', $3, $4)`,
		id, req.Status, u.ID, comment); err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось сохранить историю")
		return
	}
	_, _ = tx.Exec(ctx, `
		UPDATE invoices SET status = 'failed' WHERE request_id = $1 AND status = 'pending'`, id)
	if err := tx.Commit(ctx); err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось отменить заявку")
		return
	}

	updated, _ := getRequest(ctx, s.DB, id)
	writeJSON(w, http.StatusOK, map[string]any{"request": updated})
}

type amountInput struct {
	TotalAmount float64 `json:"total_amount"`
	Comment     string  `json:"comment"`
}

func (s *Server) handleChangeAmount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "Некорректный номер заявки")
		return
	}
	var in amountInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.TotalAmount < 0 || in.TotalAmount > 1_000_000 {
		writeFieldError(w, map[string]string{"total_amount": "Некорректная сумма"})
		return
	}
	u := currentUser(r)

	ctx, cancel := contextWithTimeout(r, 8*time.Second)
	defer cancel()

	tx, err := s.DB.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось изменить сумму")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	req, err := getRequestForUpdate(ctx, tx, id)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "not_found", "Заявка не найдена")
			return
		}
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось загрузить заявку")
		return
	}
	if req.Status == model.StatusPaid || req.Status == model.StatusCancelled {
		writeError(w, http.StatusConflict, "invalid_status", "Нельзя изменить сумму закрытой заявки")
		return
	}

	if _, err := tx.Exec(ctx, `UPDATE requests SET total_amount = $1 WHERE id = $2`, in.TotalAmount, id); err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось изменить сумму")
		return
	}
	if _, err := tx.Exec(ctx, `
		UPDATE invoices SET amount = $1 WHERE request_id = $2 AND status = 'pending'`,
		in.TotalAmount, id); err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось обновить счёт")
		return
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO request_history (request_id, from_status, to_status, changed_by, comment)
		VALUES ($1, $2::request_status, $2::request_status, $3, $4)`,
		id, req.Status, u.ID, fmt.Sprintf("Стоимость изменена на %.2f ₽. %s", in.TotalAmount, cleanString(in.Comment, 400))); err != nil {
		logDBError(s.Logger, r, err)
	}
	if err := tx.Commit(ctx); err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось изменить сумму")
		return
	}

	updated, _ := getRequest(ctx, s.DB, id)
	writeJSON(w, http.StatusOK, map[string]any{"request": updated})
}

func (s *Server) handleManagerRequests(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	q := cleanString(r.URL.Query().Get("q"), 100)
	page, limit, offset := pageParams(r)

	ctx, cancel := contextWithTimeout(r, 8*time.Second)
	defer cancel()

	where := []string{"TRUE"}
	args := []any{}
	if status == "active" {
		where = append(where, "r.status NOT IN ('paid', 'cancelled')")
	} else if status == "archive" {
		where = append(where, "r.status IN ('paid', 'cancelled')")
	} else if status != "" {
		where = append(where, fmt.Sprintf("r.status = $%d::request_status", len(args)+1))
		args = append(args, status)
	}
	if q != "" {
		where = append(where, fmt.Sprintf(
			"(r.public_number ILIKE '%%' || $%d || '%%' OR r.customer_name ILIKE '%%' || $%d || '%%' OR r.phone LIKE '%%' || $%d || '%%')",
			len(args)+1, len(args)+1, len(args)+1))
		args = append(args, q)
	}
	whereSQL := strings.Join(where, " AND ")

	var total int
	countSQL := `SELECT count(*) FROM requests r WHERE ` + whereSQL
	if err := s.DB.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось посчитать заявки")
		return
	}

	listArgs := append(append([]any{}, args...), limit, offset)
	query := requestSelect + ` WHERE ` + whereSQL + fmt.Sprintf(`
		ORDER BY CASE r.status
			WHEN 'new' THEN 0 WHEN 'accepted' THEN 1 WHEN 'assigned' THEN 2
			WHEN 'in_progress' THEN 3 WHEN 'ready' THEN 4 WHEN 'completed' THEN 5
			ELSE 6 END, r.created_at DESC
		LIMIT $%d OFFSET $%d`, len(args)+1, len(args)+2)

	rows, err := s.DB.Query(ctx, query, listArgs...)
	if err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось загрузить заявки")
		return
	}
	defer rows.Close()

	items := make([]model.Request, 0, limit)
	for rows.Next() {
		req, err := scanRequest(rows)
		if err != nil {
			logDBError(s.Logger, r, err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Ошибка чтения заявок")
			return
		}
		items = append(items, req)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": items, "total": total, "page": page, "limit": limit,
	})
}

func (s *Server) handleMasterRequests(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	status := r.URL.Query().Get("status")
	ctx, cancel := contextWithTimeout(r, 8*time.Second)
	defer cancel()

	query := requestSelect + ` WHERE r.master_id = $1`
	args := []any{u.ID}
	if status == "active" {
		query += ` AND r.status IN ('assigned', 'in_progress', 'ready')`
	} else if status != "" {
		query += ` AND r.status = $2::request_status`
		args = append(args, status)
	}
	query += ` ORDER BY CASE r.status WHEN 'assigned' THEN 0 WHEN 'in_progress' THEN 1
		WHEN 'ready' THEN 2 ELSE 3 END, r.created_at DESC`

	rows, err := s.DB.Query(ctx, query, args...)
	if err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось загрузить заявки")
		return
	}
	defer rows.Close()

	items := make([]model.Request, 0, 8)
	for rows.Next() {
		req, err := scanRequest(rows)
		if err != nil {
			logDBError(s.Logger, r, err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Ошибка чтения заявок")
			return
		}
		items = append(items, req)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}
