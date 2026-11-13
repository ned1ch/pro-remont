# REST API «pro-remont»

Базовый URL: `http://127.0.0.1:8080/api`

Аутентификация — HttpOnly-cookie `pro_session` (JWT, HS256).
Для API-клиентов поддерживается заголовок `Authorization: Bearer <token>`.
Токен выдаётся при регистрации/входе; отдельного endpoint для его получения нет.

Формат ошибок:

```json
{
  "error": {
    "code": "validation_error",
    "message": "Проверьте правильность заполнения полей",
    "fields": { "phone": "Введите корректный телефон" }
  }
}
```

Коды: `400` — валидация, `401` — нет сессии, `403` — нет прав,
`404` — не найдено, `409` — конфликт статуса/занятости, `402` — отказ банка,
`429` — слишком много попыток входа.

Для совместимости с sequence-диаграммами добавлены алиасы:
`POST /api/request` = `POST /api/requests`,
`GET /api/requests` = `GET /api/manager/requests` (только staff).

## Аутентификация и профиль

| Метод | Путь | Доступ | Описание |
|---|---|---|---|
| POST | `/auth/register` | гость | `{full_name, contact, password}`; contact — email или телефон |
| POST | `/auth/login` | гость | `{contact, password}`; ставит cookie |
| POST | `/auth/logout` | любой | очищает cookie |
| GET | `/auth/me` | любой | `{user}` или `{user:null}` для гостя |
| GET | `/profile` | авторизован | текущий профиль |
| PUT | `/profile` | авторизован | `{full_name, email, phone, address}` |

## Черновик заявки гостя

| Метод | Путь | Описание |
|---|---|---|
| POST | `/session/requisites` | `{customer_name, phone, email, address}`; сохраняет черновик на 24 часа |
| GET | `/session/requisites` | `{saved:boolean, requisites?}` по cookie `pro_draft` |

## Каталог и мастера

| Метод | Путь | Доступ | Описание |
|---|---|---|---|
| GET | `/catalog/categories` | публично | категории техники + количество услуг |
| GET | `/catalog/items?category=washer&q=насос` | публично | услуги с ценами и гарантией |
| GET | `/masters?available=true` | публично | свободные мастера |
| GET | `/masters?all=true` | manager, admin, `assign_masters` | все мастера с занятостью |

## Заявки

| Метод | Путь | Доступ | Описание |
|---|---|---|---|
| POST | `/requests` | гость/клиент | создать заявку |
| GET | `/requests/my` | клиент | активные заявки |
| GET | `/requests/my-history` | клиент | завершённые и отменённые |
| GET | `/requests/{id}` | владелец, мастер, staff | заявка + история |
| GET | `/requests/{id}/history` | владелец, мастер, staff | таймлайн |
| POST | `/requests/{id}/cancel` | владелец (new/accepted), staff | отмена |
| PATCH | `/requests/{id}/accept` | `manage_requests` | `new → accepted`, защита от гонки |
| POST | `/requests/{id}/assign-master` | `assign_masters` | `{master_id, force}` |
| PATCH | `/requests/{id}/status` | мастер/staff | `{status, comment}` |
| PATCH | `/requests/{id}/amount` | `manage_requests` | `{total_amount, comment}` |
| GET | `/manager/requests?status=active&q=` | `manage_requests` | список с фильтрами и пагинацией |
| GET | `/master/requests?status=active` | мастер | назначенные заявки |

Создание заявки:

```json
{
  "customer_name": "Иван Иванов",
  "phone": "+7 (900) 000-00-01",
  "email": "ivan@example.ru",
  "address": "г. Екатеринбург, ул. Ленина, 24, кв. 8",
  "category_id": 1,
  "service_id": 2,
  "brand_model": "Bosch WLG 2426",
  "description": "Не сливает воду, ошибка E18"
}
```

`category_id` обязателен, если не передан `service_id`; при наличии услуги
категория определяется автоматически. Для авторизованного клиента поля
профиля подставляются в пустые значения. Заявка получает номер
`PR-ГГГГ-NNNNNN`, статус `new`, а менеджеры — уведомление.

Статусы и переходы:

```
new → accepted → assigned → in_progress → ready → completed → paid
                                                ↘ cancelled
```

Мастер может выполнять только `assigned → in_progress → ready`.
Клиент может отменить только `new` или `accepted`.
Менеджер/администратор — остальные переходы, с проверкой `CanTransition`.

## Оплата

| Метод | Путь | Доступ | Описание |
|---|---|---|---|
| GET | `/billing/invoice/{request_id}` | владелец, staff | счёт по заявке (создаётся при необходимости) |
| POST | `/billing/invoice/init` | владелец, staff | `{request_id}` — инициализация платежа |
| POST | `/billing/pay` | владелец, staff | `{request_id, method, card_number}` |
| GET | `/billing/invoice/{invoice_id}/status` | владелец, staff | статус счёта |
| GET | `/billing/invoices/my` | клиент | все счета клиента |

`method`: `card`, `sbp`, `cash` (наличные — только staff).
Демо-карта `4111111111111111` успешна; номер, оканчивающийся на `0000`,
возвращает `402 payment_declined`. При оплате готовой/выполненной заявки
она переходит в `paid`. Если клиент оплатил авансом, при завершении работ
заявка автоматически станет `paid`.

## Отзывы и предложения

| Метод | Путь | Доступ | Описание |
|---|---|---|---|
| GET | `/reviews?status=approved` | публично | опубликованные отзывы + средняя оценка |
| GET | `/reviews?status=pending` | `manage_requests` | очередь модерации |
| POST | `/reviews` | авторизован | `{request_id?, rating, text, author_name?}` |
| GET | `/reviews/my` | авторизован | свои отзывы |
| PATCH | `/reviews/{id}/status` | `manage_requests` | `{status: approved\|rejected}` |
| GET | `/offers/personal` | клиент | персональные скидки по истории заказов |

Отзыв по заявке возможен только после `completed`/`paid` и только один раз.

## Уведомления и статистика

| Метод | Путь | Описание |
|---|---|---|
| GET | `/notifications?limit=50` | список + количество непрочитанных |
| POST | `/notifications/{id}/read` | отметить одно |
| POST | `/notifications/read-all` | отметить все |
| GET | `/stats` | карточки статистики по роли |

## Управление (manager/admin)

| Метод | Путь | Право | Описание |
|---|---|---|---|
| GET | `/manager/price-list` | `manage_prices` | все услуги, включая неактивные |
| PATCH | `/manager/price-list/{id}` | `manage_prices` | `{price, warranty_months, is_active, title, description}` |
| GET | `/admin/users?role=&q=` | `manage_users` | сотрудники и клиенты |
| POST | `/admin/users`, `/admin/staff` | `manage_users` | создание сотрудника; временный пароль в ответе |
| PATCH | `/admin/users/{id}` | `manage_users` | ФИО, контакты, роль, активность |
| DELETE | `/admin/users/{id}` | `manage_users` | архивирование (soft delete) |
| GET | `/admin/users/{id}/permissions` | `manage_users` | права с флагами `enabled` |
| PUT | `/admin/users/{id}/permissions` | `manage_users` | `{permissions:[...]}` |
| GET | `/admin/permissions` | `manage_users` | справочник прав |

Права: `manage_requests`, `assign_masters`, `manage_prices`,
`manage_users`, `manage_cms`, `view_reports`.
Роль `admin` всегда обладает всеми правами. Нельзя снять права
управления пользователями с себя и удалить последнего администратора.

## CMS

| Метод | Путь | Доступ | Описание |
|---|---|---|---|
| GET | `/cms/theme/config` | публично | опубликованная тема |
| GET | `/admin/theme` | `manage_cms` | черновик + опубликованная версия |
| PUT | `/admin/theme/draft` | `manage_cms` | сохранить черновик |
| POST | `/admin/theme/publish` | `manage_cms` | опубликовать черновик |

Тема:

```json
{
  "primary": "#E30613",
  "accent": "#0066CC",
  "hero_title": "Срочный ремонт бытовой техники на дому",
  "hero_subtitle": "Официальный сервисный центр...",
  "footer_note": "Профессиональный ремонт бытовой техники..."
}
```

## Служебное

`GET /api/health` — проверка соединения с БД.
