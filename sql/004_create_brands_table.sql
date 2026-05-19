-- Create brands table and link to products via brand_id

CREATE TABLE IF NOT EXISTS brands (
    id   SERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
);

INSERT INTO brands (name) VALUES
    ('NovaTech'),
    ('HomeStyle'),
    ('UrbanWear'),
    ('StyleEdge'),
    ('SipWell'),
    ('WeatherPro'),
    ('Statiq')
ON CONFLICT (name) DO NOTHING;

ALTER TABLE products ADD COLUMN IF NOT EXISTS brand_id INT REFERENCES brands(id);

UPDATE products p
SET brand_id = b.id
FROM brands b
WHERE b.name = p.product_details->>'brand';
