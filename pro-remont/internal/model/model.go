// Package model содержит доменные типы, статусы и правила переходов.
package model

import "time"

const (
	RoleClient  = "client"
	RoleMaster  = "master"
	RoleManager = "manager"
	RoleAdmin   = "admin"
)

const (
	StatusNew        = "new"
	StatusAccepted   = "accepted"
	StatusAssigned   = "assigned"
	StatusInProgress = "in_progress"
	StatusReady      = "ready"
	StatusCompleted  = "completed"
	StatusPaid       = "paid"
	StatusCancelled  = "cancelled"
)

const (
	InvoicePending = "pending"
	InvoicePaid    = "paid"
	InvoiceFailed  = "failed"
	InvoiceRefund  = "refunded"
)

const (
	ReviewPending  = "pending"
	ReviewApproved = "approved"
	ReviewRejected = "rejected"
)

// Права сотрудников. Роль admin всегда обладает всеми правами.
const (
	PermManageRequests = "manage_requests"
	PermAssignMasters  = "assign_masters"
	PermManagePrices   = "manage_prices"
	PermManageUsers    = "manage_users"
	PermManageCMS      = "manage_cms"
	PermViewReports    = "view_reports"
)

type PermissionInfo struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

var PermissionCatalog = []PermissionInfo{
	{PermManageRequests, "Обработка заявок", "Принимать заявки, менять статусы и суммы"},
	{PermAssignMasters, "Назначение мастеров", "Выбирать и закреплять мастеров за заявками"},
	{PermManagePrices, "Прайс-лист", "Изменять стоимость услуг"},
	{PermManageUsers, "Пользователи", "Создавать и удалять сотрудников, выдавать права"},
	{PermManageCMS, "Дизайн и CMS", "Менять оформление и публиковать тему сайта"},
	{PermViewReports, "Отчёты", "Смотреть сводную статистику сервиса"},
}

type User struct {
	ID          int64     `json:"id"`
	Email       string    `json:"email,omitempty"`
	Phone       string    `json:"phone,omitempty"`
	FullName    string    `json:"full_name"`
	Address     string    `json:"address,omitempty"`
	Role        string    `json:"role"`
	Permissions []string  `json:"permissions"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
}

type Master struct {
	ID                int64   `json:"id"`
	FullName          string  `json:"full_name"`
	Email             string  `json:"email,omitempty"`
	Phone             string  `json:"phone,omitempty"`
	Specialization    string  `json:"specialization"`
	Bio               string  `json:"bio"`
	ExperienceYears   int     `json:"experience_years"`
	Rating            float64 `json:"rating"`
	IsAvailable       bool    `json:"is_available"`
	ActiveRequests    int     `json:"active_requests"`
	CompletedRequests int     `json:"completed_requests"`
}

type Category struct {
	ID          int64   `json:"id"`
	Slug        string  `json:"slug"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	BasePrice   float64 `json:"base_price"`
	Services    int     `json:"services"`
}

type Service struct {
	ID             int64   `json:"id"`
	CategoryID     int64   `json:"category_id"`
	CategorySlug   string  `json:"category_slug"`
	CategoryTitle  string  `json:"category_title"`
	Title          string  `json:"title"`
	Description    string  `json:"description"`
	Price          float64 `json:"price"`
	WarrantyMonths int     `json:"warranty_months"`
	IsActive       bool    `json:"is_active"`
}

type Request struct {
	ID             int64      `json:"id"`
	PublicNumber   string     `json:"public_number"`
	ClientID       *int64     `json:"client_id,omitempty"`
	CustomerName   string     `json:"customer_name"`
	Phone          string     `json:"phone"`
	Email          string     `json:"email,omitempty"`
	Address        string     `json:"address"`
	CategoryID     *int64     `json:"category_id,omitempty"`
	CategorySlug   string     `json:"category_slug,omitempty"`
	CategoryTitle  string     `json:"category_title,omitempty"`
	ServiceID      *int64     `json:"service_id,omitempty"`
	ServiceTitle   string     `json:"service_title,omitempty"`
	EquipmentType  string     `json:"equipment_type"`
	BrandModel     string     `json:"brand_model"`
	Description    string     `json:"description"`
	Status         string     `json:"status"`
	MasterID       *int64     `json:"master_id,omitempty"`
	MasterName     string     `json:"master_name,omitempty"`
	TotalAmount    float64    `json:"total_amount"`
	ManagerComment string     `json:"manager_comment"`
	MasterComment  string     `json:"master_comment"`
	AcceptedAt     *time.Time `json:"accepted_at,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type HistoryEntry struct {
	ID         int64     `json:"id"`
	FromStatus string    `json:"from_status,omitempty"`
	ToStatus   string    `json:"to_status"`
	Comment    string    `json:"comment"`
	ChangedBy  string    `json:"changed_by,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type Review struct {
	ID         int64     `json:"id"`
	RequestID  *int64    `json:"request_id,omitempty"`
	ClientID   *int64    `json:"client_id,omitempty"`
	AuthorName string    `json:"author_name"`
	Device     string    `json:"device"`
	Rating     int       `json:"rating"`
	Text       string    `json:"text"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

type Invoice struct {
	ID          int64      `json:"id"`
	RequestID   int64      `json:"request_id"`
	Number      string     `json:"number"`
	Amount      float64    `json:"amount"`
	Status      string     `json:"status"`
	Method      string     `json:"method,omitempty"`
	CardLast4   string     `json:"card_last4,omitempty"`
	ExternalID  string     `json:"external_id,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	PaidAt      *time.Time `json:"paid_at,omitempty"`
	RequestNum  string     `json:"request_number,omitempty"`
	Customer    string     `json:"customer,omitempty"`
	Description string     `json:"description,omitempty"`
}

type InvoiceStatus struct {
	ID        int64      `json:"id"`
	Number    string     `json:"number"`
	Status    string     `json:"status"`
	Amount    float64    `json:"amount"`
	PaidAt    *time.Time `json:"paid_at,omitempty"`
	RequestID int64      `json:"request_id"`
}

type Notification struct {
	ID        int64     `json:"id"`
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	Message   string    `json:"message"`
	RequestID *int64    `json:"request_id,omitempty"`
	IsRead    bool      `json:"is_read"`
	CreatedAt time.Time `json:"created_at"`
}

type Offer struct {
	Code            string `json:"code"`
	Title           string `json:"title"`
	Description     string `json:"description"`
	DiscountPercent int    `json:"discount_percent"`
	Reason          string `json:"reason"`
}

type Theme struct {
	Primary      string `json:"primary"`
	Accent       string `json:"accent"`
	HeroTitle    string `json:"hero_title"`
	HeroSubtitle string `json:"hero_subtitle"`
	FooterNote   string `json:"footer_note"`
}

func DefaultTheme() Theme {
	return Theme{
		Primary:      "#E30613",
		Accent:       "#0066CC",
		HeroTitle:    "Срочный ремонт бытовой техники на дому в день обращения",
		HeroSubtitle: "Официальный сервисный центр ООО «pro-remont». Выезд инженера за 45 минут, оригинальные запчасти и официальная гарантия до 2 лет.",
		FooterNote:   "Профессиональный ремонт крупной и мелкой бытовой техники. Работаем ежедневно с 08:00 до 22:00.",
	}
}

var StatusLabels = map[string]string{
	StatusNew:        "Новая",
	StatusAccepted:   "Принята в обработку",
	StatusAssigned:   "Мастер назначен",
	StatusInProgress: "В работе",
	StatusReady:      "Готово к выдаче",
	StatusCompleted:  "Выполнена",
	StatusPaid:       "Оплачена",
	StatusCancelled:  "Отменена",
}

var StatusOrder = []string{
	StatusNew, StatusAccepted, StatusAssigned, StatusInProgress,
	StatusReady, StatusCompleted, StatusPaid,
}

// Допустимые переходы статусов заявки (для менеджера/администратора).
var transitions = map[string][]string{
	StatusNew:        {StatusAccepted, StatusCancelled},
	StatusAccepted:   {StatusAssigned, StatusCancelled},
	StatusAssigned:   {StatusInProgress, StatusCancelled},
	StatusInProgress: {StatusReady, StatusCancelled},
	StatusReady:      {StatusCompleted, StatusPaid, StatusCancelled},
	StatusCompleted:  {StatusPaid},
	StatusPaid:       {},
	StatusCancelled:  {},
}

func CanTransition(from, to string) bool {
	if from == to {
		return false
	}
	for _, allowed := range transitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

// CanRoleSetStatus проверяет, может ли роль перевести заявку в статус to.
func CanRoleSetStatus(role, from, to string) bool {
	if !CanTransition(from, to) {
		return false
	}
	switch role {
	case RoleAdmin:
		return true
	case RoleManager:
		switch to {
		case StatusAccepted, StatusAssigned, StatusInProgress, StatusReady, StatusCompleted, StatusPaid, StatusCancelled:
			return true
		}
	case RoleMaster:
		switch to {
		case StatusInProgress, StatusReady:
			return true
		}
	case RoleClient:
		return to == StatusCancelled && (from == StatusNew || from == StatusAccepted)
	}
	return false
}

func HasPermission(u *User, key string) bool {
	if u == nil || !u.IsActive {
		return false
	}
	if u.Role == RoleAdmin {
		return true
	}
	for _, p := range u.Permissions {
		if p == key {
			return true
		}
	}
	return false
}

func IsStaff(role string) bool {
	return role == RoleManager || role == RoleAdmin
}

func IsKnownPermission(key string) bool {
	for _, p := range PermissionCatalog {
		if p.Key == key {
			return true
		}
	}
	return false
}
