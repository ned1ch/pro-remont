// Package api содержит HTTP-маршруты, middleware и обработчики.
package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"pro-remont/internal/auth"
	"pro-remont/internal/config"
)

const sessionCookie = "pro_session"

type Server struct {
	DB           *pgxpool.Pool
	Tokens       *auth.TokenManager
	Config       config.Config
	Logger       *slog.Logger
	loginLimiter *loginLimiter
}

func New(db *pgxpool.Pool, cfg config.Config, tokens *auth.TokenManager, logger *slog.Logger) *Server {
	return &Server{
		DB:           db,
		Tokens:       tokens,
		Config:       cfg,
		Logger:       logger,
		loginLimiter: newLoginLimiter(10, 5*time.Minute),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Служебные
	mux.HandleFunc("GET /api/health", s.handleHealth)

	// Аутентификация и профиль
	mux.HandleFunc("POST /api/auth/register", s.handleRegister)
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", s.handleLogout)
	mux.HandleFunc("GET /api/auth/me", s.handleMe)
	mux.HandleFunc("GET /api/profile", s.withUser(s.handleGetProfile))
	mux.HandleFunc("PUT /api/profile", s.withUser(s.handleUpdateProfile))

	// Черновик реквизитов гостя
	mux.HandleFunc("POST /api/session/requisites", s.handleSaveRequisites)
	mux.HandleFunc("GET /api/session/requisites", s.handleGetRequisites)

	// Каталог и прайс
	mux.HandleFunc("GET /api/catalog/categories", s.handleCategories)
	mux.HandleFunc("GET /api/catalog/items", s.handleCatalogItems)
	mux.HandleFunc("GET /api/masters", s.handleMasters)

	// Отзывы
	mux.HandleFunc("GET /api/reviews", s.handleReviews)
	mux.HandleFunc("POST /api/reviews", s.withUser(s.handleCreateReview))
	mux.HandleFunc("GET /api/reviews/my", s.withUser(s.handleMyReviews))
	mux.HandleFunc("PATCH /api/reviews/{id}/status", s.withPerm("manage_requests", s.handleModerateReview))

	// Персональные предложения
	mux.HandleFunc("GET /api/offers/personal", s.withUser(s.handlePersonalOffers))

	// Заявки
	mux.HandleFunc("POST /api/requests", s.handleCreateRequest)
	// Алиас из sequence-диаграммы: POST /request.
	mux.HandleFunc("POST /api/request", s.handleCreateRequest)
	mux.HandleFunc("GET /api/requests/my", s.withUser(s.handleMyRequests))
	mux.HandleFunc("GET /api/requests/my-history", s.withUser(s.handleMyHistory))
	// Алиас из sequence-диаграммы: GET /requests (список для менеджера).
	mux.HandleFunc("GET /api/requests", s.withPerm("manage_requests", s.handleManagerRequests))
	mux.HandleFunc("GET /api/requests/{id}", s.withUser(s.handleGetRequest))
	mux.HandleFunc("GET /api/requests/{id}/history", s.withUser(s.handleRequestHistory))
	mux.HandleFunc("POST /api/requests/{id}/cancel", s.withUser(s.handleCancelRequest))
	mux.HandleFunc("PATCH /api/requests/{id}/accept", s.withPerm("manage_requests", s.handleAcceptRequest))
	mux.HandleFunc("POST /api/requests/{id}/assign-master", s.withPerm("assign_masters", s.handleAssignMaster))
	mux.HandleFunc("PATCH /api/requests/{id}/status", s.withUser(s.handleChangeStatus))
	mux.HandleFunc("PATCH /api/requests/{id}/amount", s.withPerm("manage_requests", s.handleChangeAmount))

	// Рабочие места менеджера и мастера
	mux.HandleFunc("GET /api/manager/requests", s.withPerm("manage_requests", s.handleManagerRequests))
	mux.HandleFunc("GET /api/manager/price-list", s.withPerm("manage_prices", s.handlePriceList))
	mux.HandleFunc("PATCH /api/manager/price-list/{id}", s.withPerm("manage_prices", s.handleUpdatePrice))
	mux.HandleFunc("GET /api/master/requests", s.withRole("master", s.handleMasterRequests))

	// Администрирование
	mux.HandleFunc("GET /api/admin/users", s.withPerm("manage_users", s.handleAdminUsers))
	mux.HandleFunc("POST /api/admin/users", s.withPerm("manage_users", s.handleCreateStaff))
	mux.HandleFunc("POST /api/admin/staff", s.withPerm("manage_users", s.handleCreateStaff))
	mux.HandleFunc("PATCH /api/admin/users/{id}", s.withPerm("manage_users", s.handleUpdateUser))
	mux.HandleFunc("DELETE /api/admin/users/{id}", s.withPerm("manage_users", s.handleDeleteUser))
	mux.HandleFunc("GET /api/admin/users/{id}/permissions", s.withPerm("manage_users", s.handleGetPermissions))
	mux.HandleFunc("PUT /api/admin/users/{id}/permissions", s.withPerm("manage_users", s.handleSetPermissions))
	mux.HandleFunc("GET /api/admin/permissions", s.withPerm("manage_users", s.handlePermissionCatalog))
	mux.HandleFunc("GET /api/admin/theme", s.withPerm("manage_cms", s.handleAdminTheme))
	mux.HandleFunc("PUT /api/admin/theme/draft", s.withPerm("manage_cms", s.handleSaveThemeDraft))
	mux.HandleFunc("POST /api/admin/theme/publish", s.withPerm("manage_cms", s.handlePublishTheme))

	// Оформление сайта (публично)
	mux.HandleFunc("GET /api/cms/theme/config", s.handleThemeConfig)

	// Оплата
	mux.HandleFunc("GET /api/billing/invoice/{id}", s.withUser(s.handleInvoice))
	mux.HandleFunc("GET /api/billing/invoice/{id}/status", s.withUser(s.handleInvoiceStatus))
	mux.HandleFunc("POST /api/billing/invoice/init", s.withUser(s.handleInvoiceInit))
	mux.HandleFunc("POST /api/billing/pay", s.withUser(s.handlePay))
	mux.HandleFunc("GET /api/billing/invoices/my", s.withUser(s.handleMyInvoices))

	// Уведомления и статистика
	mux.HandleFunc("GET /api/notifications", s.withUser(s.handleNotifications))
	mux.HandleFunc("POST /api/notifications/{id}/read", s.withUser(s.handleReadNotification))
	mux.HandleFunc("POST /api/notifications/read-all", s.withUser(s.handleReadAllNotifications))
	mux.HandleFunc("GET /api/stats", s.withUser(s.handleStats))

	// Неизвестные API-маршруты
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "Метод API не найден")
	})

	// Статика сайта
	mux.Handle("/", newStaticHandler(s.Config.WebDir, s.Logger))

	return s.recoverer(s.securityHeaders(s.requestID(s.accessLog(mux))))
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 3*time.Second)
	defer cancel()
	if err := s.DB.Ping(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "db_unavailable", "База данных недоступна")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"time":   time.Now().UTC(),
	})
}
