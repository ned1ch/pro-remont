package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type errorInfo struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

type errorBody struct {
	Error errorInfo `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if status == http.StatusNoContent {
		return
	}
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorBody{Error: errorInfo{Code: code, Message: message}})
}

func writeFieldError(w http.ResponseWriter, fields map[string]string) {
	writeJSON(w, http.StatusBadRequest, errorBody{Error: errorInfo{
		Code:    "validation_error",
		Message: "Проверьте правильность заполнения полей",
		Fields:  fields,
	}})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Некорректный формат запроса")
		return false
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid_json", "В запросе ожидается один JSON-объект")
		return false
	}
	return true
}

func pathID(r *http.Request, name string) (int64, bool) {
	raw := r.PathValue(name)
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func pageParams(r *http.Request) (page, limit, offset int) {
	page = 1
	if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && p > 0 {
		page = p
	}
	limit = 20
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 {
		limit = l
	}
	if limit > 100 {
		limit = 100
	}
	offset = (page - 1) * limit
	return page, limit, offset
}

func cleanString(s string, max int) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) > max {
		r := []rune(s)
		s = string(r[:max])
	}
	return s
}

func isAPIError(err error, code string) bool {
	var e *apiError
	if errors.As(err, &e) {
		return e.Code == code
	}
	return false
}

func itoa(n int) string { return strconv.Itoa(n) }

type apiError struct {
	Status  int
	Code    string
	Message string
	Fields  map[string]string
}

func (e *apiError) Error() string { return e.Message }

func badRequest(code, message string) *apiError {
	return &apiError{Status: http.StatusBadRequest, Code: code, Message: message}
}

func writeAPIError(w http.ResponseWriter, err error) {
	var e *apiError
	if errors.As(err, &e) {
		if e.Fields != nil {
			writeJSON(w, e.Status, errorBody{Error: errorInfo{Code: e.Code, Message: e.Message, Fields: e.Fields}})
			return
		}
		writeError(w, e.Status, e.Code, e.Message)
		return
	}
	writeError(w, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка сервера")
}
