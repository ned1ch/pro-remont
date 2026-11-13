package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"pro-remont/internal/auth"
	"pro-remont/internal/model"
)

// Seed создаёт демонстрационные данные, если база ещё пуста.
func (s *Store) Seed(ctx context.Context) error {
	var hasUsers bool
	if err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users)`).Scan(&hasUsers); err != nil {
		return err
	}
	if hasUsers {
		return nil
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := seedUser(ctx, tx, "admin@pro-remont.ru", "+79000000001", "Администратор", model.RoleAdmin, "Admin123!", nil); err != nil {
		return err
	}
	managerPerms := []string{model.PermManageRequests, model.PermAssignMasters, model.PermManagePrices, model.PermViewReports}
	managerID, err := seedUser(ctx, tx, "manager@pro-remont.ru", "+79000000002", "Ольга Кузнецова", model.RoleManager, "Manager123!", managerPerms)
	if err != nil {
		return err
	}
	master1ID, err := seedUser(ctx, tx, "master1@pro-remont.ru", "+79000000003", "Алексей Смирнов", model.RoleMaster, "Master123!", nil)
	if err != nil {
		return err
	}
	master2ID, err := seedUser(ctx, tx, "master2@pro-remont.ru", "+79000000004", "Дмитрий Ковалев", model.RoleMaster, "Master123!", nil)
	if err != nil {
		return err
	}
	master3ID, err := seedUser(ctx, tx, "master3@pro-remont.ru", "+79000000005", "Игорь Волков", model.RoleMaster, "Master123!", nil)
	if err != nil {
		return err
	}
	clientID, err := seedUser(ctx, tx, "client@example.ru", "+79000000006", "Елена Морозова", model.RoleClient, "Client123!", nil)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE users SET address = $1 WHERE id = $2`,
		"г. Екатеринбург, ул. Малышева, 51, кв. 12", clientID); err != nil {
		return err
	}

	masters := []struct {
		id             int64
		specialization string
		experience     int
		rating         float64
		bio            string
	}{
		{master1ID, "Ведущий инженер по холодильному оборудованию", 12, 4.9,
			"Специализируется на инверторных компрессорах и системах No Frost (Liebherr, Bosch, Samsung, LG)."},
		{master2ID, "Мастер по стиральным и сушильным машинам", 9, 4.8,
			"Выполняет сложный ремонт неразборных баков, замену подшипников и восстановление модулей управления."},
		{master3ID, "Специалист по кухонной и встраиваемой технике", 8, 4.7,
			"Ремонт индукционных варочных панелей, духовых шкафов, посудомоечных машин и кофемашин."},
	}
	for _, m := range masters {
		if _, err := tx.Exec(ctx, `
			INSERT INTO master_profiles (user_id, specialization, experience_years, rating, bio)
			VALUES ($1, $2, $3, $4, $5)`,
			m.id, m.specialization, m.experience, m.rating, m.bio); err != nil {
			return fmt.Errorf("seed master profile: %w", err)
		}
	}

	categories := []struct {
		slug        string
		title       string
		description string
		basePrice   float64
		order       int
	}{
		{"washer", "Стиральные машины", "Замена подшипников, помп, ТЭНов, ремонт электронных модулей управления, устранение протечек.", 900, 1},
		{"fridge", "Холодильники", "Заправка фреоном, замена компрессора, термостатов, датчиков оттайки и уплотнительной резины.", 1200, 2},
		{"dishwasher", "Посудомоечные машины", "Ремонт циркуляционных насосов, чистка фильтров, замена клапанов залива и нагревателей.", 1000, 3},
		{"stove", "Варочные панели и духовки", "Замена конфорок, индукционных катушек, переключателей режимов, стекол и термопар.", 1100, 4},
		{"coffee", "Кофемашины", "Декальцинация, чистка гидросистемы, ремонт заварочного блока и кофемолки.", 1300, 5},
		{"tv", "Телевизоры и СВЧ", "Замена LED-подсветки, ремонт блоков питания, замена магнетронов и слюдяных пластин.", 850, 6},
	}
	catIDs := map[string]int64{}
	for _, c := range categories {
		var id int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO categories (slug, title, description, base_price, sort_order)
			VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			c.slug, c.title, c.description, c.basePrice, c.order).Scan(&id); err != nil {
			return fmt.Errorf("seed category %s: %w", c.slug, err)
		}
		catIDs[c.slug] = id
	}

	services := []struct {
		category string
		title    string
		desc     string
		price    float64
		warranty int
		order    int
	}{
		{"washer", "Замена сливного насоса (помпы)", "Диагностика, замена помпы, проверка герметичности соединений.", 1200, 12, 1},
		{"washer", "Замена нагревательного элемента (ТЭНа)", "Замена ТЭНа с проверкой проводки и датчиков температуры.", 1100, 12, 2},
		{"washer", "Замена подшипников и сальника бака", "Разборка бака, замена подшипникового узла и сальника, сборка и тест.", 3500, 24, 3},
		{"washer", "Ремонт электронного модуля управления", "Восстановление модуля, замена силовых элементов и прошивка.", 2800, 12, 4},
		{"washer", "Устранение протечки", "Поиск и устранение причины протечки, замена уплотнений.", 900, 6, 5},
		{"fridge", "Заправка хладагентом (R134a / R600a)", "Поиск утечки, пайка, вакуумирование и заправка системы.", 2200, 12, 1},
		{"fridge", "Замена мотор-компрессора", "Замена компрессора с заменой фильтра-осушителя и заправкой.", 3200, 24, 2},
		{"fridge", "Замена датчика температуры / терморегулятора", "Диагностика цепи, замена датчика, проверка режимов.", 1400, 12, 3},
		{"fridge", "Замена уплотнительной резины", "Подбор и установка уплотнителя двери, регулировка прижима.", 1600, 6, 4},
		{"dishwasher", "Устранение засора и чистка дренажной системы", "Прочистка дренажа, чистка фильтров и проверка слива.", 1000, 6, 1},
		{"dishwasher", "Ремонт или замена циркуляционного насоса", "Замена насоса, проверка крыльчатки и герметичности.", 2400, 12, 2},
		{"dishwasher", "Замена клапана залива", "Замена впускного клапана, проверка уровня и датчиков.", 1500, 12, 3},
		{"stove", "Замена конфорки", "Замена конфорки, проверка переключателя и проводки.", 1100, 12, 1},
		{"stove", "Ремонт индукционной панели", "Диагностика силового модуля, замена катушки и элементов.", 2900, 12, 2},
		{"stove", "Замена стекла духового шкафа", "Замена жаростойкого стекла и уплотнителя двери.", 2500, 6, 3},
		{"coffee", "Декальцинация и чистка гидросистемы", "Профессиональная декальцинация, чистка заварочного блока.", 1300, 3, 1},
		{"coffee", "Ремонт заварочного блока", "Разборка, чистка и восстановление заварочного механизма.", 2100, 6, 2},
		{"tv", "Замена LED-подсветки", "Замена светодиодных планок с проверкой драйвера.", 2500, 12, 1},
		{"tv", "Ремонт блока питания", "Диагностика и восстановление блока питания телевизора.", 1800, 12, 2},
		{"tv", "Замена магнетрона СВЧ", "Замена магнетрона, проверка высоковольтной цепи.", 2200, 12, 3},
	}
	svcIDs := map[string]int64{}
	for _, sv := range services {
		var id int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO services (category_id, title, description, price, warranty_months, sort_order)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
			catIDs[sv.category], sv.title, sv.desc, sv.price, sv.warranty, sv.order).Scan(&id); err != nil {
			return fmt.Errorf("seed service %s: %w", sv.title, err)
		}
		svcIDs[sv.title] = id
	}

	now := time.Now()
	type seedReq struct {
		client       *int64
		master       *int64
		name         string
		phone        string
		email        string
		address      string
		categorySlug string
		serviceTitle string
		equipment    string
		brand        string
		description  string
		status       string
		amount       float64
		created      time.Time
	}
	reqs := []seedReq{
		{&clientID, &master2ID, "Елена Морозова", "+79000000006", "client@example.ru", "г. Екатеринбург, ул. Малышева, 51, кв. 12", "washer", "Замена сливного насоса (помпы)", "Стиральная машина", "Bosch WLG 2426", "Машина перестала сливать воду, на дне остаётся вода, ошибка E18.", model.StatusPaid, 1200, now.AddDate(0, 0, -20)},
		{&clientID, &master1ID, "Елена Морозова", "+79000000006", "client@example.ru", "г. Екатеринбург, ул. Малышева, 51, кв. 12", "fridge", "Заправка хладагентом (R134a / R600a)", "Холодильник", "Liebherr CNel 4313", "Верхняя камера перестала морозить, слышно шипение в районе компрессора.", model.StatusInProgress, 2200, now.AddDate(0, 0, -2)},
		{&clientID, &master3ID, "Елена Морозова", "+79000000006", "client@example.ru", "г. Екатеринбург, ул. Малышева, 51, кв. 12", "dishwasher", "Устранение засора и чистка дренажной системы", "Посудомоечная машина", "Electrolux ESL 94510", "Вода не уходит из камеры, горит индикатор слива.", model.StatusReady, 1000, now.AddDate(0, 0, -4)},
		{nil, nil, "Сергей Николаевич", "+79030000001", "s.nikolaev@mail.ru", "г. Екатеринбург, ул. Ленина, 24, кв. 8", "fridge", "", "Холодильник", "Atlant ХМ-4625", "Не морозит верхняя камера, на задней стенке наледь.", model.StatusNew, 1200, now.Add(-3 * time.Hour)},
		{&clientID, &master2ID, "Елена Морозова", "+79000000006", "client@example.ru", "г. Екатеринбург, ул. Малышева, 51, кв. 12", "stove", "Замена конфорки", "Варочная панель", "Bosch PKE 645", "Левая передняя конфорка не нагревается.", model.StatusCompleted, 1100, now.AddDate(0, 0, -6)},
	}

	var paidRequestID, readyRequestID, progressRequestID int64
	for i, r := range reqs {
		var id int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO requests (
				client_id, customer_name, phone, email, address, category_id, service_id,
				equipment_type, brand_model, description, status, master_id, total_amount,
				accepted_at, completed_at, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::request_status, $12, $13,
				$14, $15, $16, $16)
			RETURNING id`,
			r.client, r.name, r.phone, nullIfEmpty(r.email), r.address, catIDs[r.categorySlug], serviceID(svcIDs, r.serviceTitle),
			r.equipment, r.brand, r.description, r.status, r.master, r.amount,
			nullableTime(r.status, r.created.Add(6*time.Hour)),
			completedTime(r.status, r.created.AddDate(0, 0, 1)),
			r.created,
		).Scan(&id); err != nil {
			return fmt.Errorf("seed request %d: %w", i, err)
		}
		if r.status == model.StatusPaid {
			paidRequestID = id
		}
		if r.status == model.StatusReady {
			readyRequestID = id
		}
		if r.status == model.StatusInProgress {
			progressRequestID = id
		}

		path := statusPath(r.status)
		for j, st := range path {
			var from any
			if j > 0 {
				from = path[j-1]
			}
			actor := any(nil)
			switch st {
			case model.StatusAccepted, model.StatusAssigned, model.StatusCompleted, model.StatusPaid, model.StatusCancelled:
				actor = managerID
			case model.StatusInProgress, model.StatusReady:
				if r.master != nil {
					actor = *r.master
				} else {
					actor = managerID
				}
			}
			_, err := tx.Exec(ctx, `
				INSERT INTO request_history (request_id, from_status, to_status, changed_by, comment, created_at)
				VALUES ($1, $2::request_status, $3::request_status, $4, $5, $6)`,
				id, from, st, actor, historyComment(st), r.created.Add(time.Duration(j)*4*time.Hour))
			if err != nil {
				return fmt.Errorf("seed history: %w", err)
			}
		}
	}

	// Счета
	paidAt := now.AddDate(0, 0, -19)
	_, err = tx.Exec(ctx, `
		INSERT INTO invoices (request_id, amount, status, method, card_last4, external_id, created_at, paid_at)
		VALUES ($1, $2, 'paid', 'card', '4242', 'DEMO-PAY-0001', $3, $4)`,
		paidRequestID, 1200, now.AddDate(0, 0, -19), paidAt)
	if err != nil {
		return fmt.Errorf("seed paid invoice: %w", err)
	}
	for _, item := range []struct {
		requestID int64
		amount    float64
		created   time.Time
	}{
		{progressRequestID, 2200, now.AddDate(0, 0, -2)},
		{readyRequestID, 1000, now.AddDate(0, 0, -4)},
	} {
		if _, err := tx.Exec(ctx, `
			INSERT INTO invoices (request_id, amount, status, created_at)
			VALUES ($1, $2, 'pending', $3)`, item.requestID, item.amount, item.created); err != nil {
			return fmt.Errorf("seed pending invoice: %w", err)
		}
	}
	var completedRequestID int64
	if err := tx.QueryRow(ctx, `
		SELECT id FROM requests WHERE status = 'completed' ORDER BY id DESC LIMIT 1`).Scan(&completedRequestID); err == nil {
		_, _ = tx.Exec(ctx, `
			INSERT INTO invoices (request_id, amount, status, created_at)
			VALUES ($1, 1100, 'pending', $2)`, completedRequestID, now.AddDate(0, 0, -6))
	}

	// Отзывы
	reviews := []struct {
		requestID *int64
		clientID  *int64
		name      string
		device    string
		rating    int
		text      string
		status    string
		daysAgo   int
	}{
		{&paidRequestID, &clientID, "Елена Морозова", "Ремонт стиральной машины Bosch", 5, "Машинка перестала сливать воду прямо во время стирки. Мастер Дмитрий приехал через час после звонка, заменил насос, дал гарантийный талон. Всё чисто и аккуратно!", model.ReviewApproved, 18},
		{nil, nil, "Сергей Николаевич", "Ремонт холодильника Atlant", 5, "Перестала морозить верхняя камера. Инженер Алексей нашёл утечку фреона, запаял трубку и заправил систему. Холодильник снова работает тихо и держит температуру.", model.ReviewApproved, 12},
		{nil, nil, "Анна К.", "Ремонт посудомоечной машины Electrolux", 4, "Приехали в тот же день, прочистили слив и объяснили, как ухаживать за фильтром. Аккуратно, без лишних навязываний.", model.ReviewApproved, 9},
		{nil, nil, "Павел Д.", "Замена конфорки на варочной панели", 5, "Конфорка работала с перебоями. Мастер привёз нужную деталь с собой, заменил за полчаса. Гарантия 12 месяцев, чек прислали на почту.", model.ReviewApproved, 5},
		{nil, nil, "Марина Л.", "Ремонт кофемашины De'Longhi", 5, "Провели декальцинацию и чистку, машина снова работает как новая. Отдельное спасибо за вежливость и пунктуальность.", model.ReviewPending, 1},
	}
	for _, rv := range reviews {
		if _, err := tx.Exec(ctx, `
			INSERT INTO reviews (request_id, client_id, author_name, device, rating, text, status, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7::review_status, $8)`,
			rv.requestID, rv.clientID, rv.name, rv.device, rv.rating, rv.text, rv.status, now.AddDate(0, 0, -rv.daysAgo)); err != nil {
			return fmt.Errorf("seed review: %w", err)
		}
	}

	// Спецпредложения
	offers := []struct {
		code, title, description string
		discount, minOrders      int
		audience                 string
	}{
		{"WELCOME10", "Скидка 10% на первый ремонт", "Действует для новых клиентов сервиса при оформлении первой заявки.", 10, 0, "new"},
		{"LOYAL15", "Постоянным клиентам — 15%", "Для клиентов, которые уже воспользовались сервисом два и более раза.", 15, 2, "loyal"},
		{"CARE5", "Профилактика со скидкой 5%", "Скидка на профилактическую чистку и декальцинацию техники.", 5, 0, "all"},
	}
	for _, o := range offers {
		if _, err := tx.Exec(ctx, `
			INSERT INTO offers (code, title, description, discount_percent, audience, min_orders)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			o.code, o.title, o.description, o.discount, o.audience, o.minOrders); err != nil {
			return fmt.Errorf("seed offer: %w", err)
		}
	}

	// Уведомления
	notifications := []struct {
		userID    int64
		requestID int64
		typ       string
		title     string
		message   string
		isRead    bool
		hoursAgo  int
	}{
		{clientID, readyRequestID, "ready", "Ремонт готов к выдаче", "Посудомоечная машина Electrolux: ремонт завершён. Проверьте детали заявки и оплатите счёт.", false, 2},
		{clientID, paidRequestID, "payment", "Платёж получен", "Оплата по заявке на ремонт стиральной машины прошла успешно. Электронный чек доступен в личном кабинете.", true, 440},
		{master1ID, progressRequestID, "master_assigned", "Вам назначена заявка", "Холодильник Liebherr: заправка хладагентом. Адрес и контакты клиента — в личном кабинете.", false, 30},
		{managerID, 0, "request_created", "Новая заявка на ремонт", "Поступила заявка от гостя: холодильник Atlant не морозит. Требуется обработка.", false, 3},
	}
	for _, n := range notifications {
		var reqID any
		if n.requestID != 0 {
			reqID = n.requestID
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO notifications (user_id, request_id, type, title, message, is_read, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			n.userID, reqID, n.typ, n.title, n.message, n.isRead, now.Add(-time.Duration(n.hoursAgo)*time.Hour)); err != nil {
			return fmt.Errorf("seed notification: %w", err)
		}
	}

	theme := model.DefaultTheme()
	themeJSON, err := json.Marshal(theme)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO theme_config (id, draft, published, published_at)
		VALUES (1, $1::jsonb, $1::jsonb, now())`, string(themeJSON)); err != nil {
		return fmt.Errorf("seed theme: %w", err)
	}

	return tx.Commit(ctx)
}

func seedUser(ctx context.Context, tx pgx.Tx, email, phone, name, role, password string, perms []string) (int64, error) {
	hash, err := auth.HashPassword(password)
	if err != nil {
		return 0, err
	}
	if perms == nil {
		perms = []string{}
	}
	var id int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO users (email, phone, full_name, password_hash, role, permissions)
		VALUES ($1, $2, $3, $4, $5::user_role, $6)
		RETURNING id`,
		nullIfEmpty(email), nullIfEmpty(phone), name, hash, role, perms,
	).Scan(&id); err != nil {
		return 0, fmt.Errorf("seed user %s: %w", email, err)
	}
	return id, nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func serviceID(ids map[string]int64, title string) any {
	if title == "" {
		return nil
	}
	return ids[title]
}

func nullableTime(status string, t time.Time) any {
	switch status {
	case model.StatusNew:
		return nil
	default:
		return t
	}
}

func completedTime(status string, t time.Time) any {
	if status == model.StatusCompleted || status == model.StatusPaid {
		return t
	}
	return nil
}

func statusPath(status string) []string {
	all := []string{
		model.StatusNew, model.StatusAccepted, model.StatusAssigned,
		model.StatusInProgress, model.StatusReady, model.StatusCompleted, model.StatusPaid,
	}
	if status == model.StatusCancelled {
		return []string{model.StatusNew, model.StatusCancelled}
	}
	for i, st := range all {
		if st == status {
			return all[:i+1]
		}
	}
	return []string{model.StatusNew}
}

func historyComment(status string) string {
	switch status {
	case model.StatusNew:
		return "Заявка создана через сайт"
	case model.StatusAccepted:
		return "Менеджер принял заявку в обработку"
	case model.StatusAssigned:
		return "Назначен мастер"
	case model.StatusInProgress:
		return "Мастер приступил к ремонту"
	case model.StatusReady:
		return "Ремонт завершён, заявка готова к выдаче"
	case model.StatusCompleted:
		return "Работы выполнены"
	case model.StatusPaid:
		return "Заявка оплачена"
	case model.StatusCancelled:
		return "Заявка отменена"
	default:
		return ""
	}
}
