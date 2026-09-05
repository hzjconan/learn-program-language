-- D13 的表结构。
--
-- 应用：
--   docker exec -i golearn-pg psql -U devuser -d golearn < migrations/001_orders.sql
-- 或者用 Makefile：make db-migrate
--
-- ⚠️ 真实项目请用迁移工具（golang-migrate / goose / atlas），不要手工执行 ——
-- 迁移要能【版本化、可重放、可回滚】，讲义 §9 讲了为什么。
-- 这里只有一个文件，用最朴素的方式，把注意力留给 database/sql 本身。

DROP TABLE IF EXISTS order_items;
DROP TABLE IF EXISTS orders;

CREATE TABLE orders (
    id          bigserial PRIMARY KEY,
    customer    text        NOT NULL,
    -- note 可以为 NULL —— 练习里专门用它来练 NULL 处理（§5）
    note        text,
    status      text        NOT NULL DEFAULT 'pending',
    total_cents bigint      NOT NULL DEFAULT 0,
    created_at  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT orders_status_check
        CHECK (status IN ('pending', 'paid', 'shipped', 'cancelled')),
    CONSTRAINT orders_total_nonneg
        CHECK (total_cents >= 0)
);

CREATE TABLE order_items (
    id          bigserial PRIMARY KEY,
    order_id    bigint NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    sku         text   NOT NULL,
    qty         int    NOT NULL,
    price_cents bigint NOT NULL,

    CONSTRAINT order_items_qty_positive   CHECK (qty > 0),
    CONSTRAINT order_items_price_nonneg   CHECK (price_cents >= 0),
    -- 同一个订单里同一个 SKU 只能出现一次 —— 练习里用它来触发唯一约束冲突（§8）
    CONSTRAINT order_items_unique_sku     UNIQUE (order_id, sku)
);

CREATE INDEX orders_customer_idx ON orders (customer);
CREATE INDEX orders_status_idx   ON orders (status);

-- ⭐ 金额用 bigint 存【分】，不用 numeric/float。
-- D12 §1.1 那条的延续：float 存钱会丢精度；numeric 精确但 Go 这边没有对应的
-- 原生类型，扫出来是 string 或者要引第三方 decimal 库。存整数分最省心。
