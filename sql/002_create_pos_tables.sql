-- =============================================================
-- Migration 002: POS Tables (v2 — safe to re-run)
-- สร้าง tables ใหม่ทั้งหมด ไม่แตะ products / users ที่มีอยู่
-- รันใน Supabase SQL Editor
-- =============================================================

-- drop trigger + function ก่อนเพื่อให้ re-run ได้ปลอดภัย
DROP TRIGGER IF EXISTS trg_deduct_inventory ON orders;
DROP FUNCTION IF EXISTS deduct_inventory_on_paid();

-- -------------------------------------------------------------
-- 1. locations
-- -------------------------------------------------------------
CREATE TABLE IF NOT EXISTS locations (
    id         UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    name       VARCHAR(255) NOT NULL,
    address    TEXT,
    is_active  BOOLEAN      NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);

INSERT INTO locations (name, address)
VALUES ('สาขาหลัก', 'กรุงเทพฯ')
ON CONFLICT DO NOTHING;

-- -------------------------------------------------------------
-- 2. inventory
-- -------------------------------------------------------------
CREATE TABLE IF NOT EXISTS inventory (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id  INT         NOT NULL REFERENCES products(product_id) ON DELETE CASCADE,
    location_id UUID        NOT NULL REFERENCES locations(id) ON DELETE CASCADE,
    size        VARCHAR(20) NOT NULL DEFAULT 'ONE SIZE',
    quantity    INT         NOT NULL DEFAULT 0 CHECK (quantity >= 0),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (product_id, location_id, size)
);

CREATE INDEX IF NOT EXISTS idx_inventory_product   ON inventory(product_id);
CREATE INDEX IF NOT EXISTS idx_inventory_location  ON inventory(location_id);
CREATE INDEX IF NOT EXISTS idx_inventory_low_stock ON inventory(quantity) WHERE quantity <= 5;

-- -------------------------------------------------------------
-- 3. customers
-- -------------------------------------------------------------
CREATE TABLE IF NOT EXISTS customers (
    id         UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    name       VARCHAR(255) NOT NULL,
    phone      VARCHAR(20),
    email      VARCHAR(255),
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- -------------------------------------------------------------
-- 4. orders
-- -------------------------------------------------------------
CREATE TABLE IF NOT EXISTS orders (
    id          UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    location_id UUID          NOT NULL REFERENCES locations(id),
    cashier_id  UUID          NOT NULL REFERENCES users(id),
    customer_id UUID          REFERENCES customers(id),
    subtotal    NUMERIC(10,2) NOT NULL DEFAULT 0,
    discount    NUMERIC(10,2) NOT NULL DEFAULT 0,
    total       NUMERIC(10,2) NOT NULL DEFAULT 0,
    status      VARCHAR(20)   NOT NULL DEFAULT 'pending'
                CHECK (status IN ('pending','paid','cancelled','refunded')),
    note        TEXT,
    created_at  TIMESTAMPTZ   NOT NULL DEFAULT now(),
    paid_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_orders_location   ON orders(location_id);
CREATE INDEX IF NOT EXISTS idx_orders_cashier    ON orders(cashier_id);
CREATE INDEX IF NOT EXISTS idx_orders_status     ON orders(status);
CREATE INDEX IF NOT EXISTS idx_orders_created_at ON orders(created_at DESC);

-- -------------------------------------------------------------
-- 5. order_items
-- -------------------------------------------------------------
CREATE TABLE IF NOT EXISTS order_items (
    id           UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id     UUID          NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    product_id   INT           NOT NULL REFERENCES products(product_id),
    product_name VARCHAR(255)  NOT NULL,
    size         VARCHAR(20)   NOT NULL DEFAULT 'ONE SIZE',
    quantity     INT           NOT NULL CHECK (quantity > 0),
    unit_price   NUMERIC(10,2) NOT NULL,
    subtotal     NUMERIC(10,2) NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_order_items_order   ON order_items(order_id);
CREATE INDEX IF NOT EXISTS idx_order_items_product ON order_items(product_id);

-- -------------------------------------------------------------
-- 6. payments
-- -------------------------------------------------------------
CREATE TABLE IF NOT EXISTS payments (
    id         UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id   UUID          NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    method     VARCHAR(20)   NOT NULL
               CHECK (method IN ('cash','card','qr','transfer','other')),
    amount     NUMERIC(10,2) NOT NULL CHECK (amount > 0),
    reference  VARCHAR(255),
    created_at TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_payments_order ON payments(order_id);

-- -------------------------------------------------------------
-- 7. Trigger: ตัด inventory เมื่อ order → paid
-- -------------------------------------------------------------
CREATE OR REPLACE FUNCTION deduct_inventory_on_paid()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    v_location_id UUID;
BEGIN
    IF NEW.status = 'paid' AND (OLD.status IS DISTINCT FROM 'paid') THEN
        -- ดึง location_id จาก orders row ที่เพิ่งอัปเดต
        v_location_id := NEW.location_id;

        UPDATE inventory inv
        SET
            quantity   = inv.quantity - oi.quantity,
            updated_at = now()
        FROM order_items oi
        WHERE oi.order_id   = NEW.id
          AND inv.product_id = oi.product_id
          AND inv.location_id = v_location_id
          AND inv.size        = oi.size;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_deduct_inventory
    AFTER UPDATE OF status ON orders
    FOR EACH ROW
    EXECUTE FUNCTION deduct_inventory_on_paid();

-- -------------------------------------------------------------
-- 8. RLS
-- -------------------------------------------------------------
ALTER TABLE locations   ENABLE ROW LEVEL SECURITY;
ALTER TABLE inventory   ENABLE ROW LEVEL SECURITY;
ALTER TABLE customers   ENABLE ROW LEVEL SECURITY;
ALTER TABLE orders      ENABLE ROW LEVEL SECURITY;
ALTER TABLE order_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE payments    ENABLE ROW LEVEL SECURITY;
