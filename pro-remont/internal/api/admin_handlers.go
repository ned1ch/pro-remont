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

type adminUser struct {
	model.User
	Specialization  string  `json:"specialization,omitempty"`
	ExperienceYears int     `json:"experience_years,omitempty"`
	Rating          float64 `json:"rating,omitempty"`
	ActiveRequests  int     `json:"active_requests"`
}

func (s *Server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	role := r.URL.Query().Get("role")
	q := cleanString(r.URL.Query().Get("q"), 100)
	ctx, cancel := contextWithTimeout(r, 8*time.Second)
	defer cancel()

	rows, err := s.DB.Query(ctx, `
		SELECT u.id, COALESCE(u.email, ''), COALESCE(u.phone, ''), u.full_name, u.address,
		       u.role::text, u.permissions, u.is_active, u.created_at,
		       COALESCE(mp.specialization, ''), COALESCE(mp.experience_years, 0),
		       COALESCE(mp.rating, 0)::float8,
		       (SELECT count(*) FROM requests ar
		        WHERE ar.master_id = u.id AND ar.status IN ('assigned', 'in_progress'))
		FROM users u
		LEFT JOIN master_profiles mp ON mp.user_id = u.id
		WHERE ($1 = '' OR u.role::text = $1)
		  AND ($2 = '' OR u.full_name ILIKE '%' || $2 || '%'
		       OR COALESCE(u.email, '') ILIKE '%' || $2 || '%'
		       OR COALESCE(u.phone, '') ILIKE '%' || $2 || '%')
		ORDER BY CASE u.role WHEN 'admin' THEN 0 WHEN 'manager' THEN 1 WHEN 'master' THEN 2 ELSE 3 END,
		         u.full_name`, role, q)
	if err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось загрузить сотрудников")
		return
	}
	defer rows.Close()

	items := make([]adminUser, 0, 16)
	for rows.Next() {
		var u adminUser
		if err := rows.Scan(&u.ID, &u.Email, &u.Phone, &u.FullName, &u.Address, &u.Role,
			&u.Permissions, &u.IsActive, &u.CreatedAt, &u.Specialization,
			&u.ExperienceYears, &u.Rating, &u.ActiveRequests); err != nil {
			logDBError(s.Logger, r, err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Ошибка чтения сотрудников")
			return
		}
		if u.Permissions == nil {
			u.Permissions = []string{}
		}
		items = append(items, u)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

type createStaffInput struct {
	FullName        string   `json:"full_name"`
	Email           string   `json:"email"`
	Phone           string   `json:"phone"`
	Role            string   `json:"role"`
	Password        string   `json:"password"`
	Permissions     []string `json:"permissions"`
	Specialization  string   `json:"specialization"`
	ExperienceYears int      `json:"experience_years"`
}

func (s *Server) handleCreateStaff(w http.ResponseWriter, r *http.Request) {
	var in createStaffInput
	if !decodeJSON(w, r, &in) {
		return
	}
	actor := currentUser(r)

	fields := map[string]string{}
	name := cleanString(in.FullName, 120)
	if len([]rune(name)) < 2 {
		fields["full_name"] = "Укажите ФИО"
	}
	email, ok := auth.NormalizeEmail(in.Email)
	if !ok {
		fields["email"] = "Укажите корректный рабочий email"
	}
	role := in.Role
	if role != model.RoleMaster && role != model.RoleManager && role != model.RoleAdmin {
		fields["role"] = "Выберите роль: мастер, менеджер или администратор"
	}
	if role == model.RoleAdmin && actor.Role != model.RoleAdmin {
		fields["role"] = "Только администратор может создавать администраторов"
	}
	perms, permErr := validatePermissions(in.Permissions)
	if permErr != "" {
		fields["permissions"] = permErr
	}
	var phone string
	if strings.TrimSpace(in.Phone) != "" {
		if p, valid := auth.NormalizePhone(in.Phone); valid {
			phone = p
		} else {
			fields["phone"] = "Некорректный телефон"
		}
	}
	if in.ExperienceYears < 0 || in.ExperienceYears > 60 {
		fields["experience_years"] = "Стаж должен быть от 0 до 60 лет"
	}
	if len(fields) > 0 {
		writeFieldError(w, fields)
		return
	}

	ctx, cancel := contextWithTimeout(r, 8*time.Second)
	defer cancel()

	var exists bool
	if err := s.DB.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM users WHERE lower(email) = lower($1))`, email).Scan(&exists); err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось проверить email")
		return
	}
	if exists {
		writeFieldError(w, map[string]string{"email": "Email уже зарегистрирован в системе"})
		return
	}
	if phone != "" {
		if err := s.DB.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM users WHERE phone = $1)`, phone).Scan(&exists); err != nil {
			logDBError(s.Logger, r, err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось проверить телефон")
			return
		}
		if exists {
			writeFieldError(w, map[string]string{"phone": "Телефон уже используется"})
			return
		}
	}

	password := in.Password
	generated := false
	if password == "" {
		var err error
		password, err = auth.GeneratePassword()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось сгенерировать пароль")
			return
		}
		generated = true
	}
	if len([]rune(password)) < 6 {
		writeFieldError(w, map[string]string{"password": "Пароль должен быть не короче 6 символов"})
		return
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		writeError(w, http.StatusBadRequest, "weak_password", "Слишком короткий пароль")
		return
	}

	tx, err := s.DB.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось создать сотрудника")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id int64
	var phoneArg any
	if phone != "" {
		phoneArg = phone
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO users (email, phone, full_name, password_hash, role, permissions)
		VALUES ($1, $2, $3, $4, $5::user_role, $6)
		RETURNING id`,
		email, phoneArg, name, hash, role, perms).Scan(&id)
	if err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось создать сотрудника")
		return
	}
	if role == model.RoleMaster {
		spec := cleanString(in.Specialization, 200)
		if spec == "" {
			spec = "Мастер по ремонту бытовой техники"
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO master_profiles (user_id, specialization, experience_years)
			VALUES ($1, $2, $3)`, id, spec, in.ExperienceYears); err != nil {
			logDBError(s.Logger, r, err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось создать профиль мастера")
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось создать сотрудника")
		return
	}

	u, _ := s.loadUser(ctx, id)
	resp := map[string]any{"user": u, "message": "Учётная запись сотрудника создана"}
	if generated {
		resp["temp_password"] = password
		resp["message"] = "Учётная запись создана. Передайте сотруднику временный пароль."
	}
	writeJSON(w, http.StatusCreated, resp)
}

type updateUserInput struct {
	FullName        *string   `json:"full_name"`
	Email           *string   `json:"email"`
	Phone           *string   `json:"phone"`
	Role            *string   `json:"role"`
	Permissions     *[]string `json:"permissions"`
	IsActive        *bool     `json:"is_active"`
	Specialization  *string   `json:"specialization"`
	ExperienceYears *int      `json:"experience_years"`
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "Некорректный идентификатор")
		return
	}
	var in updateUserInput
	if !decodeJSON(w, r, &in) {
		return
	}
	actor := currentUser(r)
	ctx, cancel := contextWithTimeout(r, 8*time.Second)
	defer cancel()

	tx, err := s.DB.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось обновить пользователя")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		currentRole   string
		currentActive bool
		currentName   string
		currentEmail  string
		currentPhone  string
	)
	err = tx.QueryRow(ctx, `
		SELECT role::text, is_active, full_name, COALESCE(email, ''), COALESCE(phone, '')
		FROM users WHERE id = $1 FOR UPDATE`, id,
	).Scan(&currentRole, &currentActive, &currentName, &currentEmail, &currentPhone)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "not_found", "Пользователь не найден")
			return
		}
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось загрузить пользователя")
		return
	}

	name := currentName
	email := currentEmail
	phone := currentPhone
	role := currentRole
	active := currentActive
	perms := []string(nil)

	if in.FullName != nil {
		name = cleanString(*in.FullName, 120)
		if len([]rune(name)) < 2 {
			writeFieldError(w, map[string]string{"full_name": "Слишком короткое имя"})
			return
		}
	}
	if in.Email != nil {
		e, valid := auth.NormalizeEmail(*in.Email)
		if !valid {
			writeFieldError(w, map[string]string{"email": "Некорректный email"})
			return
		}
		email = e
	}
	if in.Phone != nil {
		if strings.TrimSpace(*in.Phone) == "" {
			phone = ""
		} else if p, valid := auth.NormalizePhone(*in.Phone); valid {
			phone = p
		} else {
			writeFieldError(w, map[string]string{"phone": "Некорректный телефон"})
			return
		}
	}
	if in.Role != nil {
		if *in.Role != model.RoleMaster && *in.Role != model.RoleManager && *in.Role != model.RoleAdmin {
			writeFieldError(w, map[string]string{"role": "Недопустимая роль"})
			return
		}
		if *in.Role == model.RoleAdmin && actor.Role != model.RoleAdmin {
			writeFieldError(w, map[string]string{"role": "Только администратор может назначать администраторов"})
			return
		}
		role = *in.Role
	}
	if in.IsActive != nil {
		active = *in.IsActive
	}
	if in.Permissions != nil {
		validated, msg := validatePermissions(*in.Permissions)
		if msg != "" {
			writeFieldError(w, map[string]string{"permissions": msg})
			return
		}
		perms = validated
	} else {
		perms = nil
	}

	// Защита от самоблокировки.
	if id == actor.ID {
		if !active {
			writeError(w, http.StatusBadRequest, "self_lockout", "Нельзя заблокировать собственную учётную запись")
			return
		}
		if role != model.RoleAdmin {
			writeError(w, http.StatusBadRequest, "self_lockout", "Нельзя снять с себя роль администратора")
			return
		}
		if in.Permissions != nil && !contains(perms, model.PermManageUsers) {
			writeError(w, http.StatusBadRequest, "self_lockout", "Нельзя лишить себя права управления пользователями")
			return
		}
	}

	// Нельзя оставить систему без активного администратора.
	if currentRole == model.RoleAdmin && (role != model.RoleAdmin || !active) {
		admins, err := activeAdmins(ctx, tx)
		if err != nil {
			logDBError(s.Logger, r, err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось проверить администраторов")
			return
		}
		if admins <= 1 {
			writeError(w, http.StatusConflict, "last_admin", "Нельзя удалить или разжаловать последнего администратора")
			return
		}
	}

	if email != "" {
		var exists bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM users WHERE lower(email) = lower($1) AND id <> $2)`, email, id).Scan(&exists); err != nil {
			logDBError(s.Logger, r, err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось проверить email")
			return
		}
		if exists {
			writeFieldError(w, map[string]string{"email": "Email уже используется"})
			return
		}
	}
	if phone != "" {
		var exists bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM users WHERE phone = $1 AND id <> $2)`, phone, id).Scan(&exists); err != nil {
			logDBError(s.Logger, r, err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось проверить телефон")
			return
		}
		if exists {
			writeFieldError(w, map[string]string{"phone": "Телефон уже используется"})
			return
		}
	}

	var emailArg, phoneArg any
	if email != "" {
		emailArg = email
	}
	if phone != "" {
		phoneArg = phone
	}

	if in.Permissions != nil {
		_, err = tx.Exec(ctx, `
			UPDATE users SET full_name = $1, email = $2, phone = $3, role = $4::user_role,
			                 permissions = $5, is_active = $6
			WHERE id = $7`, name, emailArg, phoneArg, role, perms, active, id)
	} else {
		_, err = tx.Exec(ctx, `
			UPDATE users SET full_name = $1, email = $2, phone = $3, role = $4::user_role,
			                 is_active = $5
			WHERE id = $6`, name, emailArg, phoneArg, role, active, id)
	}
	if err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось обновить пользователя")
		return
	}

	if role == model.RoleMaster {
		if in.ExperienceYears != nil && (*in.ExperienceYears < 0 || *in.ExperienceYears > 60) {
			writeFieldError(w, map[string]string{"experience_years": "Стаж должен быть от 0 до 60 лет"})
			return
		}
		if in.Specialization != nil || in.ExperienceYears != nil {
			spec := ""
			if in.Specialization != nil {
				spec = cleanString(*in.Specialization, 200)
			}
			exp := 0
			if in.ExperienceYears != nil {
				exp = *in.ExperienceYears
			}
			if _, err = tx.Exec(ctx, `
				INSERT INTO master_profiles (user_id, specialization, experience_years)
				VALUES ($1, NULLIF($2, ''), $3)
				ON CONFLICT (user_id) DO NOTHING`,
				id, spec, exp); err != nil {
				logDBError(s.Logger, r, err)
				writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось создать профиль мастера")
				return
			}
			if in.Specialization != nil {
				_, err = tx.Exec(ctx, `UPDATE master_profiles SET specialization = $1 WHERE user_id = $2`, spec, id)
			}
			if in.ExperienceYears != nil {
				_, err = tx.Exec(ctx, `UPDATE master_profiles SET experience_years = $1 WHERE user_id = $2`, exp, id)
			}
			if err != nil {
				logDBError(s.Logger, r, err)
				writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось обновить профиль мастера")
				return
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось обновить пользователя")
		return
	}

	updated, _ := s.loadUser(ctx, id)
	writeJSON(w, http.StatusOK, map[string]any{"user": updated})
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "Некорректный идентификатор")
		return
	}
	actor := currentUser(r)
	if id == actor.ID {
		writeError(w, http.StatusBadRequest, "self_lockout", "Нельзя удалить собственную учётную запись")
		return
	}

	ctx, cancel := contextWithTimeout(r, 8*time.Second)
	defer cancel()

	tx, err := s.DB.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось удалить пользователя")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var role string
	var active bool
	err = tx.QueryRow(ctx, `SELECT role::text, is_active FROM users WHERE id = $1 FOR UPDATE`, id).Scan(&role, &active)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "not_found", "Пользователь не найден")
			return
		}
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось загрузить пользователя")
		return
	}
	if role == model.RoleAdmin {
		admins, err := activeAdmins(ctx, tx)
		if err != nil {
			logDBError(s.Logger, r, err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось проверить администраторов")
			return
		}
		if admins <= 1 {
			writeError(w, http.StatusConflict, "last_admin", "Нельзя удалить последнего администратора")
			return
		}
	}
	var activeRequests int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM requests WHERE master_id = $1 AND status IN ('assigned', 'in_progress')`,
		id).Scan(&activeRequests); err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось проверить заявки мастера")
		return
	}
	if activeRequests > 0 {
		writeError(w, http.StatusConflict, "master_has_requests",
			fmt.Sprintf("У мастера %d активных заявок. Сначала переназначьте их.", activeRequests))
		return
	}

	if _, err := tx.Exec(ctx, `UPDATE users SET is_active = FALSE, permissions = '{}' WHERE id = $1`, id); err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось удалить пользователя")
		return
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO notifications (user_id, type, title, message)
		VALUES ($1, 'system', 'Учётная запись отключена', 'Доступ к системе закрыт администратором.')`, id); err != nil {
		// пользователь может быть недоступен — не критично
	}
	if err := tx.Commit(ctx); err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось удалить пользователя")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Учётная запись переведена в архив"})
}

func (s *Server) handleGetPermissions(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "Некорректный идентификатор")
		return
	}
	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()

	var role string
	var perms []string
	err := s.DB.QueryRow(ctx, `SELECT role::text, permissions FROM users WHERE id = $1`, id).Scan(&role, &perms)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "not_found", "Пользователь не найден")
			return
		}
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось загрузить права")
		return
	}
	if perms == nil {
		perms = []string{}
	}
	type permState struct {
		model.PermissionInfo
		Enabled bool `json:"enabled"`
	}
	out := make([]permState, 0, len(model.PermissionCatalog))
	for _, p := range model.PermissionCatalog {
		enabled := role == model.RoleAdmin || contains(perms, p.Key)
		out = append(out, permState{PermissionInfo: p, Enabled: enabled})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user_id": id, "role": role, "permissions": out,
		"is_admin": role == model.RoleAdmin,
	})
}

func (s *Server) handleSetPermissions(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "Некорректный идентификатор")
		return
	}
	var in struct {
		Permissions []string `json:"permissions"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	perms, msg := validatePermissions(in.Permissions)
	if msg != "" {
		writeFieldError(w, map[string]string{"permissions": msg})
		return
	}
	actor := currentUser(r)
	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()

	var role string
	if err := s.DB.QueryRow(ctx, `SELECT role::text FROM users WHERE id = $1`, id).Scan(&role); err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "not_found", "Пользователь не найден")
			return
		}
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось загрузить пользователя")
		return
	}
	if id == actor.ID && role != model.RoleAdmin && !contains(perms, model.PermManageUsers) {
		writeError(w, http.StatusBadRequest, "self_lockout", "Невозможно лишить себя прав управления пользователями")
		return
	}
	if _, err := s.DB.Exec(ctx, `UPDATE users SET permissions = $1 WHERE id = $2`, perms, id); err != nil {
		logDBError(s.Logger, r, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось сохранить права")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "permissions": perms, "message": "Права успешно обновлены",
	})
}

func (s *Server) handlePermissionCatalog(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"permissions": model.PermissionCatalog})
}

func activeAdmins(ctx context.Context, tx pgx.Tx) (int, error) {
	var n int
	err := tx.QueryRow(ctx, `SELECT count(*) FROM users WHERE role = 'admin' AND is_active`).Scan(&n)
	return n, err
}

func validatePermissions(perms []string) ([]string, string) {
	out := make([]string, 0, len(perms))
	seen := map[string]bool{}
	for _, p := range perms {
		if !model.IsKnownPermission(p) {
			return nil, "Неизвестное право доступа: " + p
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out, ""
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}
