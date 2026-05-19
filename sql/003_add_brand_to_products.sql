-- Add brand field to product_details JSONB
-- Brand assignment based on product category:
--   NovaTech   → Tech/Electronics (keyboards, mice, headphones, chargers, lamps)
--   HomeStyle  → Furniture/Home (chairs, desks, mats)
--   UrbanWear  → Clothing (hoodies, shirts, jackets, socks, sneakers, gloves, belts)
--   StyleEdge  → Accessories (scarves, sunglasses, totes, wallets)
--   SipWell    → Drinkware (tumblers, mugs, bottles)
--   WeatherPro → Outdoor/Travel (umbrellas)
--   Statiq     → Stationery (notebooks, pencil cases)

-- NovaTech: Tech peripherals & electronics
UPDATE products
SET product_details = jsonb_set(product_details, '{brand}', '"NovaTech"')
WHERE product_name IN (
    'Luxury Keyboard', 'Travel Keyboard',
    'Compact Mouse', 'Sport Mouse', 'Classic Mouse', 'Eco Mouse',
    'Eco Headphones', 'Premium Headphones', 'Vintage Headphones', 'Smart Headphones',
    'Vintage Charger', 'Classic Charger',
    'Smart Lamp', 'Vintage Lamp'
);

-- HomeStyle: Furniture & home
UPDATE products
SET product_details = jsonb_set(product_details, '{brand}', '"HomeStyle"')
WHERE product_name IN (
    'Smart Chair', 'Classic Chair', 'Sport Chair',
    'Classic Mat', 'Compact Mat',
    'Compact Desk'
);

-- UrbanWear: Clothing & footwear
UPDATE products
SET product_details = jsonb_set(product_details, '{brand}', '"UrbanWear"')
WHERE product_name IN (
    'Compact Hoodie', 'Sport Hoodie', 'Premium Hoodie',
    'Vintage Shirt',
    'Sport Socks', 'Smart Socks',
    'Sport Gloves',
    'Smart Sneakers',
    'Eco Jacket',
    'Modern Belt'
);

-- StyleEdge: Fashion accessories
UPDATE products
SET product_details = jsonb_set(product_details, '{brand}', '"StyleEdge"')
WHERE product_name IN (
    'Classic Scarf', 'Vintage Scarf', 'Travel Scarf', 'Compact Scarf',
    'Modern Tote', 'Luxury Tote',
    'Travel Wallet', 'Smart Wallet',
    'Compact Sunglasses'
);

-- SipWell: Drinkware
UPDATE products
SET product_details = jsonb_set(product_details, '{brand}', '"SipWell"')
WHERE product_name IN (
    'Smart Tumbler', 'Classic Tumbler',
    'Vintage Mug',
    'Vintage Bottle'
);

-- WeatherPro: Outdoor & umbrellas
UPDATE products
SET product_details = jsonb_set(product_details, '{brand}', '"WeatherPro"')
WHERE product_name IN (
    'Eco Umbrella', 'Sport Umbrella', 'Compact Umbrella', 'Vintage Umbrella'
);

-- Statiq: Stationery
UPDATE products
SET product_details = jsonb_set(product_details, '{brand}', '"Statiq"')
WHERE product_name IN (
    'Modern Notebook',
    'Smart Pencil Case', 'Compact Pencil Case'
);
