package api

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"time"

	"pro-remont/internal/model"
)

var hexColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func (s *Server) handleThemeConfig(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()

	theme, err := s.loadTheme(ctx, "published")
	if err != nil {
		theme = model.DefaultTheme()
	}
	writeJSON(w, http.StatusOK, theme)
}

func (s *Server) handleAdminTheme(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()

	var draftRaw, publishedRaw []byte
	var updatedAt time.Time
	var publishedAt *time.Time
	err := s.DB.QueryRow(ctx, `
		SELECT draft, published, updated_at, published_at FROM theme_config WHERE id = 1`,
	).Scan(&draftRaw, &publishedRaw, &updatedAt, &publishedAt)
	if err != nil {
		if !isNotFound(err) {
			logDBError(s.Logger, r, err)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"draft": model.DefaultTheme(), "published": model.DefaultTheme(),
			"updated_at": nil, "published_at": nil,
		})
		return
	}
	var draft, published model.Theme
	_ = json.Unmarshal(draftRaw, &draft)
	_ = json.Unmarshal(publishedRaw, &published)
	if draft.Primary == "" {
		draft = model.DefaultTheme()
	}
	if published.Primary == "" {
		published = model.DefaultTheme()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"draft": draft, "published": published,
		"updated_at": updatedAt, "published_at": publishedAt,
	})
}

func (s *Server) handleSaveThemeDraft(w http.ResponseWriter, r *http.Request) {
	var in model.Theme
	if !decodeJSON(w, r, &in) {
		return
	}
	if fields := validateTheme(in); len(fields) > 0 {
		writeFieldError(w, fields)
		return
	}
	raw, err := json.Marshal(in)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось сохранить тему")
		return
	}

	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()

	_, err = s.DB.Exec(ctx, `
		INSERT INTO theme_config (id, draft, published)
		VALUES (1, $1::jsonb, $1::jsonb)
		ON CONFLICT (id) DO UPDATE SET draft = $1::jsonb, updated_at = now()`, string(raw))
	if err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось сохранить черновик темы")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"draft": in, "message": "Черновик оформления сохранён",
	})
}

func (s *Server) handlePublishTheme(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 8*time.Second)
	defer cancel()

	var draftRaw []byte
	if err := s.DB.QueryRow(ctx, `SELECT draft FROM theme_config WHERE id = 1`).Scan(&draftRaw); err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "not_found", "Черновик темы не найден")
			return
		}
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось загрузить тему")
		return
	}
	var draft model.Theme
	if err := json.Unmarshal(draftRaw, &draft); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Повреждён черновик темы")
		return
	}
	if fields := validateTheme(draft); len(fields) > 0 {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: errorInfo{
			Code: "validation_error", Message: "Черновик содержит ошибки", Fields: fields,
		}})
		return
	}

	raw, _ := json.Marshal(draft)
	_, err := s.DB.Exec(ctx, `
		UPDATE theme_config SET published = $1::jsonb, published_at = now(), updated_at = now()
		WHERE id = 1`, string(raw))
	if err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось опубликовать тему")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"published": draft, "message": "Изменения опубликованы. Обновите страницу сайта.",
	})
}

func (s *Server) loadTheme(ctx context.Context, column string) (model.Theme, error) {
	var raw []byte
	query := `SELECT published FROM theme_config WHERE id = 1`
	if column == "draft" {
		query = `SELECT draft FROM theme_config WHERE id = 1`
	}
	if err := s.DB.QueryRow(ctx, query).Scan(&raw); err != nil {
		return model.Theme{}, err
	}
	var theme model.Theme
	if err := json.Unmarshal(raw, &theme); err != nil {
		return model.Theme{}, err
	}
	return theme, nil
}

func validateTheme(t model.Theme) map[string]string {
	fields := map[string]string{}
	if !hexColorRe.MatchString(t.Primary) {
		fields["primary"] = "Укажите цвет в формате #RRGGBB"
	}
	if !hexColorRe.MatchString(t.Accent) {
		fields["accent"] = "Укажите цвет в формате #RRGGBB"
	}
	if l := len([]rune(t.HeroTitle)); l < 10 || l > 200 {
		fields["hero_title"] = "Заголовок должен быть от 10 до 200 символов"
	}
	if l := len([]rune(t.HeroSubtitle)); l < 10 || l > 400 {
		fields["hero_subtitle"] = "Подзаголовок должен быть от 10 до 400 символов"
	}
	if l := len([]rune(t.FooterNote)); l < 10 || l > 300 {
		fields["footer_note"] = "Текст подвала должен быть от 10 до 300 символов"
	}
	return fields
}
