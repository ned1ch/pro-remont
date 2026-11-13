# Архитектура и соответствие диаграммам

## Слои приложения

```
Браузер
  ├── HTML-страницы (web/*.html)
  ├── CSS (web/assets/css/style.css)
  └── ES-модули (web/assets/js)
        ├── api.js      — fetch + ApiError
        ├── ui.js       — модальные окна, тосты, форматирование, бейджи
        ├── app.js      — шапка/подвал, сессия, тема CMS, уведомления
        ├── payments.js — оплата и электронный чек
        └── pages/*.js  — логика конкретной страницы
                 │  JSON / HttpOnly cookie
                 ▼
Go net/http (internal/api)
  ├── middleware: request-id, лог, security headers, recover, auth
  ├── handlers: auth, catalog, requests, billing, reviews, admin, CMS
  ├── model: статусы, переходы, права
  └── store: pgxpool, миграции, seed
                 │ SQL
                 ▼
PostgreSQL
  users, requests, invoices, reviews, notifications, ...
```

Frontend не требует сборки: браузер загружает ES-модули напрямую.
Сервер отдаёт статику из `web/` и JSON по `/api/*`, все SPA-подобные
переходы кабинета выполняются через hash (`account.html#requests`)
и query-параметры (`?request=ID`).

## Аутентификация и разграничение доступа

1. `POST /api/auth/login` проверяет bcrypt-хеш и выдаёт JWT (HS256, 24 часа).
2. JWT кладётся в HttpOnly-cookie `pro_session` (SameSite=Lax).
3. Middleware `withUser` загружает пользователя из БД на каждый запрос,
   поэтому блокировка и смена прав применяются сразу.
4. `withPerm` проверяет право; роль `admin` обходит проверку.
5. Для мастера дополнительно проверяется, что заявка назначена именно ему.
6. Для клиента — что заявка принадлежит ему (`client_id`).

## Машина статусов

```
new ──accept──► accepted ──assign──► assigned ──start──► in_progress
                                                              │
                                                          finish
                                                              ▼
paid ◄──pay/complete── completed ◄──complete── ready ◄───────┘
  ▲
  └── оплата авансом: при завершении заявка сразу становится paid
```

Переходы описаны в `internal/model/model.go` (`CanTransition`,
`CanRoleSetStatus`). Любое изменение пишется в `request_history` и,
для ключевых событий, создаёт запись в `notifications`.

## Соответствие 29 страницам drawio

| Диаграмма | Реализация |
|---|---|
| `use-case` | роли и сценарии ниже |
| Оформить заявку на ремонт | `POST /api/requests`, `contacts.html`, валидация 400/201 |
| Принять и обработать заявку | `PATCH /api/requests/{id}/accept`, защита от гонки через `FOR UPDATE` |
| Назначать мастера | `POST /api/requests/{id}/assign-master`, `GET /masters?all=true`, проверка занятости |
| Обновлять статус заявки | `PATCH /api/requests/{id}/status`, история, уведомления |
| Управлять пользователями и правами | `/api/admin/users`, `/permissions`, панель администратора |
| Зарегистрироваться на сайте | `POST /api/auth/register`, модальное окно, проверка уникальности |
| Авторизоваться на сайте | `POST /api/auth/login`, rate-limit, cookie-сессия |
| Редактировать профиль | `GET/PUT /api/profile`, вкладка «Профиль» |
| Получать спец.предложения | `GET /api/offers/personal`, подбор по истории заказов |
| Посмотреть статус ремонта | `GET /api/requests/my`, карточки и таймлайн |
| Получить уведомление о готовности | `notifications` + polling в `app.js`, модальное окно «Готово» |
| Оплатить заказ | `POST /api/billing/pay`, счёт, чек |
| Просмотреть каталоги | `GET /api/catalog/categories`, `GET /api/catalog/items` |
| Просмотреть отзывы | `GET /api/reviews?status=approved` |
| Изменять цены на услуги | `GET /api/manager/price-list`, `PATCH .../{id}` |
| Переделать дизайн и структуру сайта | CMS: `theme_config`, черновик/публикация, CSS-переменные |
| Указать реквизиты | `POST/GET /api/session/requisites`, cookie-черновик |
| Удалять учётные записи сотрудников | `DELETE /api/admin/users/{id}` (soft delete, защита последнего админа) |
| Настраивать права доступа | `GET/PUT /api/admin/users/{id}/permissions` |
| Создавать учётные записи сотрудников | `POST /api/admin/users`, временный пароль |
| Просматривать список заявок | `GET /api/manager/requests?status=&q=` |
| Просматривать список назначенных заявок | `GET /api/master/requests` |
| Оплатить заказ (менеджер) | `POST /api/billing/pay` с `method=cash`, доступ по `manage_requests` |
| `гость` | главная, прайс, мастера, отзывы, заявка, регистрация/вход |
| `клиент` | кабинет: заявки, история, оплата, отзывы, предложения, профиль |
| `мастер` | назначенные заявки, начало/завершение ремонта |
| `Менеджер` | все заявки, приём, назначение, оплата, прайс, модерация |
| `Администратор` | пользователи, права, CMS, полный доступ |

## Последовательности из диаграмм

| Диаграмма (sequence) | Endpoint |
|---|---|
| Создание заявки | `POST /request` → `/api/requests` |
| Обработка заявки | `GET /requests`, `PATCH /requests/{id}/accept` |
| Назначение мастера | `GET /masters?available=true`, `POST /requests/{id}/assign-master` |
| Статусы мастера | `GET /requests/my`, `PATCH /requests/{id}/status` |
| Администрирование | `GET/POST /admin/users`, `DELETE /admin/users/{id}`, `GET/PUT /admin/users/{id}/permissions` |
| Регистрация/вход | `POST /auth/register`, `POST /auth/login` |
| Профиль | `GET/PUT /profile` |
| Спецпредложения | `GET /offers/personal` |
| Статус ремонта и уведомление | `GET /requests/my-history`, `GET /requests/{id}`, notifications |
| Оплата | `GET /billing/invoice/{request_id}`, `POST /billing/pay` |
| Каталог | `GET /catalog/categories`, `GET /catalog/items` |
| Отзывы | `GET /reviews?status=approved` |
| Прайс менеджера | `GET/PATCH /manager/price-list` |
| CMS | `GET /cms/theme/config`, `POST /cms/theme/publish` |
| Реквизиты гостя | `POST /session/requisites` |

## Обработка ошибок и конкурентность

- Валидация возвращает `400` с картой `fields`, фронтенд подсвечивает поля.
- Принятие заявки — `UPDATE ... WHERE status='new'` в транзакции
  с `SELECT ... FOR UPDATE`; второй менеджер получает `409 already_taken`.
- Назначение мастера проверяет роль, активность, ручную доступность
  и количество активных заявок; при занятости требуется `force=true`.
- Все изменения статусов и оплат выполняются в транзакциях.
- Пароли — bcrypt cost 10; JWT подписывается секретом из `.env`.
