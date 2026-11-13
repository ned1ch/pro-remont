-- Схема БД сервисного центра «pro-remont»
-- PostgreSQL 14+

CREATE TYPE user_role AS ENUM ('client', 'master', 'manager', 'admin');
CREATE TYPE request_status AS ENUM (
    'new', 'accepted', 'assigned', 'in_progress',
    'ready', 'completed', 'paid', 'cancelled'
);
CREATE TYPE invoice_status AS ENUM ('pending', 'paid', 'failed', 'refunded');
CREATE TYPE review_status AS ENUM ('pending', 'approved', 'rejected');

CREATE OR REPLACE FUNCTION set_updated_at() RETURNS trigger AS $$
BEGIN
    NEW.updated_at := now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TABLE users (
    id             BIGSERIAL PRIMARY KEY,
    email          TEXT,
    phone          TEXT,
    full_name      TEXT NOT NULL CHECK (length(full_name) BETWEEN 2 AND 120),
    address        TEXT NOT NULL DEFAULT '',
    password_hash  TEXT NOT NULL,
    role           user_role NOT NULL DEFAULT 'client',
    permissions    TEXT[] NOT NULL DEFAULT '{}',
    is_active      BOOLEAN NOT NULL DEFAULT TRUE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT users_contact_check CHECK (email IS NOT NULL OR phone IS NOT NULL)
);

CREATE UNIQUE INDEX users_email_lower_uq ON users (lower(email)) WHERE email IS NOT NULL;
CREATE UNIQUE INDEX users_phone_uq ON users (phone) WHERE phone IS NOT NULL;
CREATE INDEX users_role_idx ON users (role) WHERE is_active;

CREATE TRIGGER users_set_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE master_profiles (
    user_id           BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    specialization    TEXT NOT NULL DEFAULT '',
    experience_years  INT NOT NULL DEFAULT 0 CHECK (experience_years >= 0 AND experience_years <= 60),
    rating            NUMERIC(2,1) NOT NULL DEFAULT 5.0 CHECK (rating >= 0 AND rating <= 5),
    bio               TEXT NOT NULL DEFAULT '',
    is_available      BOOLEAN NOT NULL DEFAULT TRUE,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE categories (
    id           BIGSERIAL PRIMARY KEY,
    slug         TEXT NOT NULL UNIQUE,
    title        TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    base_price   NUMERIC(10,2) NOT NULL DEFAULT 0 CHECK (base_price >= 0),
    sort_order   INT NOT NULL DEFAULT 0
);

CREATE TABLE services (
    id               BIGSERIAL PRIMARY KEY,
    category_id      BIGINT NOT NULL REFERENCES categories(id) ON DELETE RESTRICT,
    title            TEXT NOT NULL,
    description      TEXT NOT NULL DEFAULT '',
    price            NUMERIC(10,2) NOT NULL CHECK (price >= 0),
    warranty_months  INT NOT NULL DEFAULT 0 CHECK (warranty_months >= 0),
    is_active        BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order       INT NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX services_category_idx ON services (category_id);

CREATE TRIGGER services_set_updated_at
    BEFORE UPDATE ON services
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE requests (
    id               BIGSERIAL PRIMARY KEY,
    public_number    TEXT NOT NULL UNIQUE,
    client_id        BIGINT REFERENCES users(id) ON DELETE SET NULL,
    customer_name    TEXT NOT NULL CHECK (length(customer_name) BETWEEN 2 AND 120),
    phone            TEXT NOT NULL,
    email            TEXT,
    address          TEXT NOT NULL DEFAULT '',
    category_id      BIGINT REFERENCES categories(id) ON DELETE SET NULL,
    service_id       BIGINT REFERENCES services(id) ON DELETE SET NULL,
    equipment_type   TEXT NOT NULL DEFAULT '',
    brand_model      TEXT NOT NULL DEFAULT '',
    description      TEXT NOT NULL CHECK (length(description) BETWEEN 5 AND 4000),
    status           request_status NOT NULL DEFAULT 'new',
    master_id        BIGINT REFERENCES users(id) ON DELETE SET NULL,
    total_amount     NUMERIC(10,2) NOT NULL DEFAULT 0 CHECK (total_amount >= 0),
    manager_comment  TEXT NOT NULL DEFAULT '',
    master_comment   TEXT NOT NULL DEFAULT '',
    accepted_at      TIMESTAMPTZ,
    completed_at     TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX requests_client_idx ON requests (client_id, created_at DESC);
CREATE INDEX requests_master_idx ON requests (master_id, status);
CREATE INDEX requests_status_idx ON requests (status, created_at DESC);

CREATE OR REPLACE FUNCTION assign_request_number() RETURNS trigger AS $$
BEGIN
    NEW.public_number := 'PR-' || to_char(COALESCE(NEW.created_at, now()), 'YYYY')
        || '-' || lpad(NEW.id::text, 6, '0');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER requests_assign_number
    BEFORE INSERT ON requests
    FOR EACH ROW EXECUTE FUNCTION assign_request_number();

CREATE TRIGGER requests_set_updated_at
    BEFORE UPDATE ON requests
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE request_history (
    id           BIGSERIAL PRIMARY KEY,
    request_id   BIGINT NOT NULL REFERENCES requests(id) ON DELETE CASCADE,
    from_status  request_status,
    to_status    request_status NOT NULL,
    changed_by   BIGINT REFERENCES users(id) ON DELETE SET NULL,
    comment      TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX request_history_request_idx ON request_history (request_id, created_at);

CREATE TABLE invoices (
    id           BIGSERIAL PRIMARY KEY,
    number       TEXT NOT NULL UNIQUE,
    request_id   BIGINT NOT NULL UNIQUE REFERENCES requests(id) ON DELETE CASCADE,
    amount       NUMERIC(10,2) NOT NULL CHECK (amount >= 0),
    status       invoice_status NOT NULL DEFAULT 'pending',
    method       TEXT,
    card_last4   TEXT,
    external_id  TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    paid_at      TIMESTAMPTZ
);

CREATE OR REPLACE FUNCTION assign_invoice_number() RETURNS trigger AS $$
BEGIN
    NEW.number := 'INV-' || to_char(COALESCE(NEW.created_at, now()), 'YYYY')
        || '-' || lpad(NEW.id::text, 6, '0');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER invoices_assign_number
    BEFORE INSERT ON invoices
    FOR EACH ROW EXECUTE FUNCTION assign_invoice_number();

CREATE TABLE reviews (
    id            BIGSERIAL PRIMARY KEY,
    request_id    BIGINT REFERENCES requests(id) ON DELETE SET NULL,
    client_id     BIGINT REFERENCES users(id) ON DELETE SET NULL,
    author_name   TEXT NOT NULL,
    device        TEXT NOT NULL DEFAULT '',
    rating        INT NOT NULL CHECK (rating BETWEEN 1 AND 5),
    text          TEXT NOT NULL CHECK (length(text) BETWEEN 10 AND 4000),
    status        review_status NOT NULL DEFAULT 'pending',
    moderated_by  BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX reviews_status_idx ON reviews (status, created_at DESC);
CREATE UNIQUE INDEX reviews_request_uq ON reviews (request_id) WHERE request_id IS NOT NULL;

CREATE TABLE offers (
    id                BIGSERIAL PRIMARY KEY,
    code              TEXT NOT NULL UNIQUE,
    title             TEXT NOT NULL,
    description       TEXT NOT NULL DEFAULT '',
    discount_percent  INT NOT NULL CHECK (discount_percent BETWEEN 0 AND 100),
    audience          TEXT NOT NULL DEFAULT 'all' CHECK (audience IN ('all', 'new', 'loyal')),
    min_orders        INT NOT NULL DEFAULT 0,
    is_active         BOOLEAN NOT NULL DEFAULT TRUE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE notifications (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    request_id  BIGINT REFERENCES requests(id) ON DELETE CASCADE,
    type        TEXT NOT NULL DEFAULT 'system',
    title       TEXT NOT NULL,
    message     TEXT NOT NULL DEFAULT '',
    is_read     BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX notifications_user_idx ON notifications (user_id, is_read, created_at DESC);

CREATE TABLE theme_config (
    id            SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    draft         JSONB NOT NULL,
    published     JSONB NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at  TIMESTAMPTZ
);

CREATE TABLE session_drafts (
    sid         TEXT PRIMARY KEY,
    data        JSONB NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX session_drafts_expires_idx ON session_drafts (expires_at);
