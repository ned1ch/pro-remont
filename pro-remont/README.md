# ООО «pro-remont» — сайт сервисного центра

Полноценное веб-приложение для сервисного центра по ремонту бытовой техники:
публичный сайт, личные кабинеты клиента и мастера, рабочее место менеджера
и панель администратора.

Проект собран по готовой вёрстке (`добычка lame`) и 29 диаграммам из
`добычин_диаграммы_курсачвсе4.drawio`: use-case, activity- и sequence-диаграммы
ролей «гость», «клиент», «мастер», «менеджер», «администратор».

![Главная страница](docs/screenshots/home.png)

## Стек

| Слой | Технологии |
|---|---|
| Backend | Go 1.26+, стандартный `net/http` с method-routing, `pgx/v5` |
| База данных | PostgreSQL 14+ (enum-статусы, триггеры, JSONB для темы, `TEXT[]` для прав) |
| Аутентификация | JWT (HS256) в HttpOnly-cookie, bcrypt, роли и granular permissions |
| Frontend | Vanilla JavaScript (ES modules), HTML5, CSS3 — без сборщика и npm |
| Инфраструктура | SQL-миграции, идемпотентный seed, PowerShell-скрипты, Docker Compose для БД |

## Что реализовано

### Гость
- главная страница, каталог техники, прайс-лист с поиском и фильтрами;
- карточки мастеров, отзывы клиентов;
- оформление заявки без регистрации;
- автосохранение черновика реквизитов (`POST /api/session/requisites`);
- регистрация и вход по email или телефону.

### Клиент
- личный кабинет со статистикой;
- активные заявки и история ремонтов;
- статусы и подробная история изменений;
- оплата картой / СБП / наличными (демо-шлюз) и электронные чеки;
- персональные спецпредложения;
- отзывы после выполненных работ;
- уведомления о готовности и оплате;
- редактирование профиля.

### Мастер
- список назначенных заявок с контактами и адресом;
- переходы «Начать работу» → «Завершить ремонт»;
- уведомление о назначении заявки.

### Менеджер
- все заявки с фильтрами по статусу и поиском;
- приём заявки в обработку;
- назначение мастера с проверкой занятости и принудительным подтверждением;
- полный жизненный цикл статусов, изменение суммы, приём оплаты;
- редактирование прайс-листа (цена, гарантия, активность);
- модерация отзывов.

### Администратор
- всё, что доступно менеджеру;
- создание сотрудников с временным паролем;
- выдача и отзыв прав доступа;
- блокировка и архивирование учётных записей;
- защита от самоблокировки и удаления последнего администратора;
- CMS: черновик и публикация темы (цвета, заголовки, текст подвала).

## Быстрый старт (Windows)

### Вариант 1. Локальная учебная БД (рекомендуется)

Нужен установленный PostgreSQL 14+ и Go 1.26+.

```powershell
cd pro-remont
powershell -ExecutionPolicy Bypass -File scripts\run.ps1
```

Скрипт:
1. создаст `.env`, если его нет;
2. инициализирует отдельный кластер в `pro-remont\.pgdata` на порту **55432**
   (системный сервис PostgreSQL на 5432 не затрагивается);
3. создаст базу `pro_remont`;
4. запустит сервер на <http://127.0.0.1:8080>.

> `initdb` нельзя запускать из терминала с правами администратора — используйте
> обычную сессию пользователя.

### Вариант 2. Свой PostgreSQL

1. Создайте базу:
   ```sql
   CREATE DATABASE pro_remont;
   ```
2. Скопируйте `.env.example` в `.env` и укажите свой `DATABASE_URL`, например:
   ```
   DATABASE_URL=postgres://postgres:password@127.0.0.1:5432/pro_remont?sslmode=disable
   JWT_SECRET=длинная-случайная-строка
   ```
3. Запустите:
   ```bash
   go mod download
   go run ./cmd/server
   ```

Миграции и демо-данные применяются автоматически при старте.

### Вариант 3. PostgreSQL в Docker

```bash
docker compose up -d db
```

Используйте строку подключения из `docker-compose.yml`:
`postgres://postgres:postgres@127.0.0.1:55433/pro_remont?sslmode=disable`.

## Демо-доступы

| Роль | Логин | Пароль |
|---|---|---|
| Клиент | `client@example.ru` | `Client123!` |
| Мастер (холодильники) | `master1@pro-remont.ru` | `Master123!` |
| Мастер (стиральные машины) | `master2@pro-remont.ru` | `Master123!` |
| Мастер (кухонная техника) | `master3@pro-remont.ru` | `Master123!` |
| Менеджер | `manager@pro-remont.ru` | `Manager123!` |
| Администратор | `admin@pro-remont.ru` | `Admin123!` |

Можно входить по email или по телефону (`+79000000006` для клиента).

## Структура проекта

```
pro-remont/
├── cmd/server/main.go              # точка входа, graceful shutdown
├── internal/
│   ├── api/                        # маршруты, middleware, обработчики
│   ├── auth/                       # bcrypt, JWT, нормализация контактов
│   ├── config/                     # .env + переменные окружения
│   ├── model/                      # доменные типы, статусы, права
│   └── store/
│       ├── migrations/001_init.sql # схема БД
│       ├── migrate.go              # встроенные миграции (embed)
│       └── seed.go                 # демо-данные
├── web/
│   ├── assets/css/style.css
│   ├── assets/js/
│   │   ├── api.js                  # fetch-обёртка
│   │   ├── ui.js                   # модальные окна, тосты, форматирование
│   │   ├── app.js                  # шапка, подвал, auth, тема, уведомления
│   │   ├── payments.js             # окно оплаты и чек
│   │   └── pages/                  # скрипты страниц
│   ├── index.html, prices.html, masters.html, reviews.html,
│   ├── about.html, contacts.html, account.html, 404.html
├── docs/
│   ├── API.md
│   ├── DATABASE.md
│   ├── ARCHITECTURE.md
│   ├── diagrams.drawio             # исходные 29 диаграмм
│   └── screenshots/
├── scripts/
│   ├── dev-db.ps1                  # локальный кластер PostgreSQL
│   └── run.ps1                     # БД + сервер
├── docker-compose.yml
├── .env.example
└── go.mod
```

## API

Полное описание — в [docs/API.md](docs/API.md). Кратко:

- `POST /api/auth/register`, `POST /api/auth/login`, `GET /api/auth/me`;
- `GET /api/catalog/categories`, `GET /api/catalog/items`;
- `GET /api/masters?available=true|all=true`;
- `POST /api/requests`, `GET /api/requests/my`, `GET /api/requests/{id}`;
- `PATCH /api/requests/{id}/accept`, `POST /api/requests/{id}/assign-master`,
  `PATCH /api/requests/{id}/status`;
- `GET /api/manager/requests`, `GET/PATCH /api/manager/price-list`;
- `GET/POST/PATCH/DELETE /api/admin/users`, `/api/admin/users/{id}/permissions`;
- `GET /api/billing/invoice/{request_id}`, `POST /api/billing/pay`;
- `GET /api/reviews`, `POST /api/reviews`, `PATCH /api/reviews/{id}/status`;
- `GET /api/notifications`, `GET /api/stats`;
- `GET /api/cms/theme/config`, `PUT /api/admin/theme/draft`,
  `POST /api/admin/theme/publish`.

Маршруты соответствуют sequence-диаграммам из drawio
(`POST /request`, `PATCH /requests/{id}/accept`, `POST /requests/{id}/assign-master`,
`PATCH /requests/{id}/status`, `POST /auth/register`, `PUT /profile`,
`GET /offers/personal`, `GET /billing/invoice/{request_id}` и т.д.).
Подробная карта «диаграмма → код» — в [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## База данных

Схема и связи — в [docs/DATABASE.md](docs/DATABASE.md).

Основные таблицы: `users`, `master_profiles`, `categories`, `services`,
`requests`, `request_history`, `invoices`, `reviews`, `offers`,
`notifications`, `theme_config`, `session_drafts`.

Статусы заявки:

```
new → accepted → assigned → in_progress → ready → completed → paid
                                                ↘ cancelled
```

Переходы проверяются на сервере с учётом роли; все изменения пишутся
в `request_history`.

## Демо-оплата

Платёжный шлюз имитируется, реальные деньги не списываются:

- карта `4111 1111 1111 1111` — успешная оплата;
- карта, оканчивающаяся на `0000`, — отказ банка;
- СБП — успешная оплата;
- наличные — доступны менеджеру/администратору.

После оплаты создаётся электронный чек с номером операции.

## Тесты и проверка

```bash
go test ./...
go vet ./...
```

Юнит-тесты покрывают переходы статусов, матрицу ролей, права доступа,
нормализацию телефонов/email и bcrypt.

Сквозные сценарии проверены в headless Chrome: регистрация/вход, создание
заявки, принятие менеджером, назначение мастера, смена статусов мастером,
завершение, оплата, отзывы, права администратора, мобильное меню.

## Безопасность

- пароли хранятся только в bcrypt-хешах;
- JWT в HttpOnly + SameSite=Lax cookie, срок жизни 24 часа;
- SQL-запросы параметризованы;
- роли и granular permissions проверяются на каждом защищённом маршруте;
- защита от самоблокировки и удаления последнего администратора;
- CSP, `X-Frame-Options`, `X-Content-Type-Options`, `Referrer-Policy`;
- пользовательский ввод экранируется на фронтенде;
- ограничение попыток входа (10 за 5 минут на IP).

## Ограничения учебного проекта

- оплата — демо-имитация без реального эквайринга;
- SMS/email-уведомления не отправляются, уведомления хранятся в БД и видны в кабинете;
- CMS меняет цвета и тексты темы, а не файловую структуру шаблонов;
- для production нужно вынести `JWT_SECRET` в секреты, включить HTTPS и настроить резервное копирование БД.

## Скриншоты

| | |
|---|---|
| ![Главная](docs/screenshots/home.png) | ![Кабинет клиента](docs/screenshots/account-client.png) |
| ![Заявка](docs/screenshots/request-detail.png) | ![Панель менеджера](docs/screenshots/account-manager.png) |
| ![Оплата](docs/screenshots/payment.png) | ![Мобильная версия](docs/screenshots/mobile.png) |

