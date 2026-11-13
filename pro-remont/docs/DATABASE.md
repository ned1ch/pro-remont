# База данных «pro-remont»

СУБД: PostgreSQL 14+. Схема создаётся миграцией
`internal/store/migrations/001_init.sql` при старте сервера.

## ER-диаграмма

```mermaid
erDiagram
    users ||--o| master_profiles : "профиль мастера"
    users ||--o{ requests : "клиент"
    users ||--o{ requests : "мастер"
    categories ||--o{ services : "содержит"
    categories ||--o{ requests : "тип техники"
    services ||--o{ requests : "услуга"
    requests ||--o{ request_history : "история"
    requests ||--o{ invoices : "счёт"
    requests ||--o{ reviews : "отзыв"
    users ||--o{ notifications : "получает"
    users ||--o{ reviews : "автор"
    offers }o--o{ users : "персонально"

    users {
        bigint id PK
        text email
        text phone
        text full_name
        text address
        text password_hash
        user_role role
        text[] permissions
        boolean is_active
        timestamptz created_at
        timestamptz updated_at
    }
    master_profiles {
        bigint user_id PK,FK
        text specialization
        int experience_years
        numeric rating
        text bio
        boolean is_available
    }
    categories {
        bigint id PK
        text slug
        text title
        text description
        numeric base_price
    }
    services {
        bigint id PK
        bigint category_id FK
        text title
        numeric price
        int warranty_months
        boolean is_active
    }
    requests {
        bigint id PK
        text public_number
        bigint client_id FK
        text customer_name
        text phone
        text address
        bigint category_id FK
        bigint service_id FK
        text description
        request_status status
        bigint master_id FK
        numeric total_amount
        timestamptz created_at
        timestamptz completed_at
    }
    request_history {
        bigint id PK
        bigint request_id FK
        request_status from_status
        request_status to_status
        bigint changed_by FK
        text comment
    }
    invoices {
        bigint id PK
        text number
        bigint request_id FK
        numeric amount
        invoice_status status
        text method
        text card_last4
        timestamptz paid_at
    }
    reviews {
        bigint id PK
        bigint request_id FK
        bigint client_id FK
        text author_name
        int rating
        text text
        review_status status
    }
    notifications {
        bigint id PK
        bigint user_id FK
        bigint request_id FK
        text type
        text title
        boolean is_read
    }
```

## Перечисления

| Тип | Значения |
|---|---|
| `user_role` | `client`, `master`, `manager`, `admin` |
| `request_status` | `new`, `accepted`, `assigned`, `in_progress`, `ready`, `completed`, `paid`, `cancelled` |
| `invoice_status` | `pending`, `paid`, `failed`, `refunded` |
| `review_status` | `pending`, `approved`, `rejected` |

## Таблицы

### `users`
Учётные записи всех ролей. Контакт — email и/или телефон (CHECK).
Уникальные индексы по `lower(email)` и `phone`. Пароль — bcrypt.
`permissions` — массив прав для сотрудников (администратор всегда имеет все).
`is_active=false` — архивирование при удалении.

### `master_profiles`
Дополнительные данные мастера: специализация, стаж, рейтинг, биография,
ручная доступность. Доступность для назначения вычисляется как
`is_available AND нет активных заявок`.

### `categories` и `services`
Категории техники (стиральные машины, холодильники и т.д.) и услуги
прайс-листа. `base_price` категории используется, если клиент не выбрал
конкретную услугу. `warranty_months` — гарантия на работу.

### `requests`
Заявка на ремонт. Может быть создана гостем (`client_id IS NULL`) или
клиентом. Номер `public_number` формируется триггером `PR-ГГГГ-NNNNNN`.
`status` меняется только через API с проверкой допустимых переходов.
`total_amount` фиксируется при создании и может быть изменена менеджером.

### `request_history`
Аудит статусов: кто, когда, из какого в какой статус и с каким комментарием
перевёл заявку. Используется для таймлайна в карточке заявки.

### `invoices`
Счёт по заявке, один к одному (`UNIQUE request_id`). Номер `INV-ГГГГ-NNNNNN`
формируется триггером. Хранит способ оплаты, последние 4 цифры карты,
код операции и время оплаты.

### `reviews`
Отзывы клиентов. Для отзыва по заявке — уникальный индекс
`reviews_request_uq`, поэтому повторный отзыв по одной заявке невозможен.
Новые отзывы попадают в `pending` и публикуются после модерации.

### `offers`
Скидки: `audience` = `all` / `new` / `loyal`, `min_orders` — порог по истории.
Персональные предложения вычисляются в `GET /api/offers/personal`.

### `notifications`
Внутренние уведомления пользователям: назначение мастера, готовность,
оплата, новые заявки для менеджеров. `is_read` — флаг прочтения.

### `theme_config`
Одна строка (`id=1`) с JSONB-черновиком и опубликованной темой CMS
(цвета, заголовки, текст подвала).

### `session_drafts`
Временные черновики реквизитов гостя на 24 часа, привязаны к cookie
`pro_draft` (HttpOnly).

## Триггеры и индексы

- `set_updated_at` — обновляет `updated_at` у `users`, `services`, `requests`;
- `assign_request_number` — генерирует номер заявки до вставки;
- `assign_invoice_number` — генерирует номер счёта;
- индексы по клиенту, мастеру, статусу и датам создания заявок;
- индекс по `notifications(user_id, is_read, created_at)`;
- индекс по `request_history(request_id, created_at)`;
- частичный уникальный индекс по email (без учёта регистра) и телефону.

## Демо-данные

Seed выполняется только если таблица `users` пуста:
6 пользователей (админ, менеджер, 3 мастера, клиент), 6 категорий,
20 услуг, 5 заявок в разных статусах, счета, 5 отзывов, 3 спецпредложения,
уведомления и тема по умолчанию.

Сбросить базу:

```powershell
# остановить сервер, затем
dropdb -h 127.0.0.1 -p 55432 -U postgres --force pro_remont
createdb -h 127.0.0.1 -p 55432 -U postgres pro_remont
# запустить сервер — миграции и seed применятся заново
```
