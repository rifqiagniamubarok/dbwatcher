-- Test schema for DBWatch manual and integration testing.

CREATE TABLE IF NOT EXISTS users (
    id         serial PRIMARY KEY,
    name       text NOT NULL,
    email      text,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS orders (
    id         serial PRIMARY KEY,
    user_id    integer NOT NULL,
    total      numeric(10,2),
    status     text NOT NULL DEFAULT 'pending',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS order_items (
    id         serial PRIMARY KEY,
    order_id   integer NOT NULL,
    product_id integer NOT NULL,
    qty        integer NOT NULL DEFAULT 1,
    price      numeric(10,2)
);

CREATE TABLE IF NOT EXISTS inventory (
    id         serial PRIMARY KEY,
    product_id integer NOT NULL UNIQUE,
    stock      integer NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- Enable full replica identity so UPDATE/DELETE carry old values.
ALTER TABLE users      REPLICA IDENTITY FULL;
ALTER TABLE orders     REPLICA IDENTITY FULL;
ALTER TABLE order_items REPLICA IDENTITY FULL;
ALTER TABLE inventory  REPLICA IDENTITY FULL;
