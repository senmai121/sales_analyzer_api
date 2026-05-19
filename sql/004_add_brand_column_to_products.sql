-- Add brand as a dedicated column on products table
-- Copy existing brand values from product_details JSONB

ALTER TABLE products ADD COLUMN IF NOT EXISTS brand TEXT;

UPDATE products
SET brand = product_details->>'brand'
WHERE brand IS NULL AND product_details->>'brand' IS NOT NULL;
