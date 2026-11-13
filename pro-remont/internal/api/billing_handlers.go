package api

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"pro-remont/internal/model"
)

const invoiceSelect = `
	SELECT i.id, i.number, i.request_id, i.amount::float8, i.status::text,
	       COALESCE(i.method, ''), COALESCE(i.card_last4, ''), i.external_id,
	       i.created_at, i.paid_at, r.public_number, r.customer_name, r.description
	FROM invoices i
	JOIN requests r ON r.id = i.request_id`

func scanInvoice(sc rowScanner) (model.Invoice, error) {
	var inv model.Invoice
	err := sc.Scan(
		&inv.ID, &inv.Number, &inv.RequestID, &inv.Amount, &inv.Status,
		&inv.Method, &inv.CardLast4, &inv.ExternalID, &inv.CreatedAt, &inv.PaidAt,
		&inv.RequestNum, &inv.Customer, &inv.Description,
	)
	return inv, err
}

func getInvoiceByID(ctx context.Context, q requestQuerier, id int64) (*model.Invoice, error) {
	inv, err := scanInvoice(q.QueryRow(ctx, invoiceSelect+` WHERE i.id = $1`, id))
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

func getInvoiceByRequest(ctx context.Context, q requestQuerier, requestID int64) (*model.Invoice, error) {
	inv, err := scanInvoice(q.QueryRow(ctx, invoiceSelect+` WHERE i.request_id = $1`, requestID))
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

func (s *Server) ensureInvoice(ctx context.Context, requestID int64) (*model.Invoice, error) {
	var amount float64
	if err := s.DB.QueryRow(ctx, `SELECT total_amount::float8 FROM requests WHERE id = $1`, requestID).Scan(&amount); err != nil {
		return nil, err
	}
	_, err := s.DB.Exec(ctx, `
		INSERT INTO invoices (request_id, amount) VALUES ($1, $2)
		ON CONFLICT (request_id) DO NOTHING`, requestID, amount)
	if err != nil {
		return nil, err
	}
	return getInvoiceByRequest(ctx, s.DB, requestID)
}

func (s *Server) handleInvoice(w http.ResponseWriter, r *http.Request) {
	requestID, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "Некорректный номер заявки")
		return
	}
	u := currentUser(r)

	ctx, cancel := contextWithTimeout(r, 8*time.Second)
	defer cancel()

	req, err := getRequest(ctx, s.DB, requestID)
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
		writeError(w, http.StatusForbidden, "forbidden", "Нет доступа к счёту")
		return
	}

	inv, err := s.ensureInvoice(ctx, requestID)
	if err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось сформировать счёт")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"invoice": inv,
		"request": req,
		"lines": []map[string]any{
			{"title": invoiceLineTitle(req), "amount": inv.Amount},
		},
	})
}

func invoiceLineTitle(req *model.Request) string {
	if req.ServiceTitle != "" {
		return req.ServiceTitle
	}
	if req.CategoryTitle != "" {
		return "Диагностика и ремонт: " + req.CategoryTitle
	}
	return "Ремонт бытовой техники"
}

func (s *Server) handleInvoiceStatus(w http.ResponseWriter, r *http.Request) {
	invoiceID, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "Некорректный номер счёта")
		return
	}
	u := currentUser(r)

	ctx, cancel := contextWithTimeout(r, 8*time.Second)
	defer cancel()

	inv, err := getInvoiceByID(ctx, s.DB, invoiceID)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "not_found", "Счёт не найден")
			return
		}
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось загрузить счёт")
		return
	}
	req, err := getRequest(ctx, s.DB, inv.RequestID)
	if err != nil || !canViewRequest(u, req) {
		writeError(w, http.StatusForbidden, "forbidden", "Нет доступа к счёту")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"invoice": inv})
}

func (s *Server) handleInvoiceInit(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RequestID int64 `json:"request_id"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.RequestID <= 0 {
		writeFieldError(w, map[string]string{"request_id": "Укажите заявку"})
		return
	}
	u := currentUser(r)

	ctx, cancel := contextWithTimeout(r, 8*time.Second)
	defer cancel()

	req, err := getRequest(ctx, s.DB, in.RequestID)
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
		writeError(w, http.StatusForbidden, "forbidden", "Нет доступа к заявке")
		return
	}
	inv, err := s.ensureInvoice(ctx, in.RequestID)
	if err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось сформировать счёт")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"invoice":    inv,
		"payment_id": fmt.Sprintf("PAY-%d-%d", inv.ID, time.Now().Unix()),
	})
}

type payInput struct {
	RequestID  int64  `json:"request_id"`
	Method     string `json:"method"`
	CardNumber string `json:"card_number"`
	Comment    string `json:"comment"`
}

func (s *Server) handlePay(w http.ResponseWriter, r *http.Request) {
	var in payInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.RequestID <= 0 {
		writeFieldError(w, map[string]string{"request_id": "Укажите заявку"})
		return
	}
	switch in.Method {
	case "card", "sbp", "cash":
	default:
		writeFieldError(w, map[string]string{"method": "Выберите способ оплаты"})
		return
	}

	u := currentUser(r)
	isStaff := model.HasPermission(u, model.PermManageRequests)

	ctx, cancel := contextWithTimeout(r, 10*time.Second)
	defer cancel()

	req, err := getRequest(ctx, s.DB, in.RequestID)
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
	if !isOwner && !isStaff {
		writeError(w, http.StatusForbidden, "forbidden", "Нет доступа к заявке")
		return
	}
	if in.Method == "cash" && !isStaff {
		writeError(w, http.StatusForbidden, "forbidden", "Оплата наличными принимается менеджером")
		return
	}
	if req.Status == model.StatusCancelled {
		writeError(w, http.StatusConflict, "invalid_status", "Нельзя оплатить отменённую заявку")
		return
	}

	inv, err := s.ensureInvoice(ctx, in.RequestID)
	if err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось сформировать счёт")
		return
	}
	if inv.Status == model.InvoicePaid {
		writeError(w, http.StatusConflict, "already_paid", "Счёт уже оплачен")
		return
	}

	digits := onlyDigits(in.CardNumber)
	last4 := ""
	declined := false
	if in.Method == "card" {
		if len(digits) < 16 || len(digits) > 19 {
			writeFieldError(w, map[string]string{"card_number": "Введите 16–19 цифр номера карты"})
			return
		}
		last4 = digits[len(digits)-4:]
		// Демо-шлюз: карта, оканчивающаяся на 0000, имитирует отказ банка.
		declined = strings.HasSuffix(digits, "0000")
	}

	tx, err := s.DB.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось провести платёж")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var currentStatus string
	if err := tx.QueryRow(ctx, `SELECT status::text FROM invoices WHERE id = $1 FOR UPDATE`, inv.ID).Scan(&currentStatus); err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось проверить счёт")
		return
	}
	if currentStatus == model.InvoicePaid {
		writeError(w, http.StatusConflict, "already_paid", "Счёт уже оплачен")
		return
	}

	if declined {
		_, _ = tx.Exec(ctx, `
			UPDATE invoices SET status = 'failed', method = $1, card_last4 = $2, external_id = $3
			WHERE id = $4`,
			in.Method, last4, "DECLINED-"+randomID(), inv.ID)
		_ = tx.Commit(ctx)
		writeError(w, http.StatusPaymentRequired, "payment_declined", "Банк отклонил платёж: недостаточно средств или карта не поддерживается")
		return
	}

	externalID := "DEMO-" + strings.ToUpper(in.Method) + "-" + randomID()
	var last4Arg any
	if last4 != "" {
		last4Arg = last4
	}
	if _, err := tx.Exec(ctx, `
		UPDATE invoices
		SET status = 'paid', method = $1, card_last4 = $2, external_id = $3, paid_at = now()
		WHERE id = $4`, in.Method, last4Arg, externalID, inv.ID); err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось провести платёж")
		return
	}

	if model.CanTransition(req.Status, model.StatusPaid) {
		if _, err := tx.Exec(ctx, `UPDATE requests SET status = 'paid' WHERE id = $1`, req.ID); err != nil {
			logDBError(s.Logger, r, err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось обновить заявку")
			return
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO request_history (request_id, from_status, to_status, changed_by, comment)
			VALUES ($1, $2::request_status, 'paid', $3, $4)`,
			req.ID, req.Status, u.ID, "Платёж получен: "+paymentLabel(in.Method)+". Счёт "+inv.Number); err != nil {
			logDBError(s.Logger, r, err)
		}
	} else {
		_, _ = tx.Exec(ctx, `
			INSERT INTO request_history (request_id, from_status, to_status, changed_by, comment)
			VALUES ($1, $2::request_status, $2::request_status, $3, $4)`,
			req.ID, req.Status, u.ID, "Оплачен счёт "+inv.Number+" ("+paymentLabel(in.Method)+")")
	}

	if req.ClientID != nil {
		_, _ = tx.Exec(ctx, `
			INSERT INTO notifications (user_id, request_id, type, title, message)
			VALUES ($1, $2, 'payment', 'Платёж получен', $3)`,
			*req.ClientID, req.ID, fmt.Sprintf("Оплата %s по заявке %s прошла успешно. Чек доступен в личном кабинете.", formatRub(inv.Amount), req.PublicNumber))
	}
	if isStaff {
		_, _ = tx.Exec(ctx, `
			INSERT INTO notifications (user_id, request_id, type, title, message)
			VALUES ($1, $2, 'payment', 'Принята оплата', $3)`,
			u.ID, req.ID, fmt.Sprintf("Заявка %s оплачена (%s).", req.PublicNumber, paymentLabel(in.Method)))
	}

	if err := tx.Commit(ctx); err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось провести платёж")
		return
	}

	updatedInv, _ := getInvoiceByRequest(ctx, s.DB, req.ID)
	updatedReq, _ := getRequest(ctx, s.DB, req.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"invoice": updatedInv,
		"request": updatedReq,
		"message": "Платёж успешно проведён",
	})
}

func (s *Server) handleMyInvoices(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	ctx, cancel := contextWithTimeout(r, 8*time.Second)
	defer cancel()

	rows, err := s.DB.Query(ctx, invoiceSelect+`
		WHERE r.client_id = $1
		ORDER BY i.created_at DESC`, u.ID)
	if err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось загрузить счета")
		return
	}
	defer rows.Close()

	items := make([]model.Invoice, 0, 8)
	for rows.Next() {
		inv, err := scanInvoice(rows)
		if err != nil {
			logDBError(s.Logger, r, err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Ошибка чтения счетов")
			return
		}
		items = append(items, inv)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func paymentLabel(method string) string {
	switch method {
	case "card":
		return "банковская карта"
	case "sbp":
		return "СБП"
	case "cash":
		return "наличные"
	default:
		return method
	}
}

func formatRub(v float64) string {
	if v == math.Trunc(v) {
		return groupThousands(strconv.FormatInt(int64(v), 10)) + " ₽"
	}
	return strings.Replace(strconv.FormatFloat(v, 'f', 2, '.'), ".", ",", 1) + " ₽"
}

func groupThousands(digits string) string {
	negative := strings.HasPrefix(digits, "-")
	if negative {
		digits = digits[1:]
	}
	var b strings.Builder
	for i, ch := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(ch)
	}
	if negative {
		return "-" + b.String()
	}
	return b.String()
}
