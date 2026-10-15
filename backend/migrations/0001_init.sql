CREATE TABLE items (
    item_id        TEXT PRIMARY KEY,
    name           TEXT        NOT NULL,
    total_stock    INTEGER     NOT NULL CHECK (total_stock >= 0),
    reserved_stock INTEGER     NOT NULL DEFAULT 0 CHECK (reserved_stock >= 0),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT reserved_not_exceed_total CHECK (reserved_stock <= total_stock)
);

CREATE TYPE reservation_status AS ENUM ('ACTIVE', 'CONFIRMED', 'EXPIRED');

CREATE TABLE reservations (
    reservation_id TEXT PRIMARY KEY,
    user_id        TEXT        NOT NULL,
    item_id        TEXT        NOT NULL REFERENCES items(item_id),
    quantity       INTEGER     NOT NULL CHECK (quantity > 0),
    status         reservation_status NOT NULL DEFAULT 'ACTIVE',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at     TIMESTAMPTZ NOT NULL,
    confirmed_at   TIMESTAMPTZ
);

CREATE INDEX idx_reservations_due
    ON reservations (expires_at) WHERE status = 'ACTIVE';

CREATE INDEX idx_reservations_item_status
    ON reservations (item_id, status);

INSERT INTO items (item_id, name, total_stock) VALUES ('item_4021', 'Flash Sale Widget', 100);
