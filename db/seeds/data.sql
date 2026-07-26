-- Admin user (password: admin123456)
INSERT INTO users (email, password_hash, first_name, last_name, role, active) VALUES
('admin@example.com', '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy', 'Admin', 'User', 'admin', true)
ON CONFLICT (email) DO NOTHING;

-- Sample categories
INSERT INTO categories (name, slug) VALUES
('Electronics', 'electronics'),
('Clothing', 'clothing'),
('Books', 'books'),
('Home & Garden', 'home-garden'),
('Sports', 'sports')
ON CONFLICT (slug) DO NOTHING;

-- Sample products
INSERT INTO products (name, slug, description, price, currency, sku, category_id, status) VALUES
('Wireless Headphones', 'wireless-headphones', 'Premium wireless headphones with noise cancellation', 9999, 'USD', 'SEED-ELEC-001', (SELECT id FROM categories WHERE slug = 'electronics'), 'published'),
('Bluetooth Speaker', 'bluetooth-speaker', 'Portable bluetooth speaker with deep bass', 4999, 'USD', 'SEED-ELEC-002', (SELECT id FROM categories WHERE slug = 'electronics'), 'published'),
('USB-C Hub', 'usb-c-hub', '7-in-1 USB-C hub with HDMI and ethernet', 3499, 'USD', 'SEED-ELEC-003', (SELECT id FROM categories WHERE slug = 'electronics'), 'published'),
('Cotton T-Shirt', 'cotton-tshirt', 'Comfortable 100% cotton t-shirt', 1999, 'USD', 'SEED-CLOTH-001', (SELECT id FROM categories WHERE slug = 'clothing'), 'published'),
('Denim Jeans', 'denim-jeans', 'Classic fit denim jeans', 4999, 'USD', 'SEED-CLOTH-002', (SELECT id FROM categories WHERE slug = 'clothing'), 'published'),
('Running Shoes', 'running-shoes', 'Lightweight running shoes with cushioned sole', 7999, 'USD', 'SEED-CLOTH-003', (SELECT id FROM categories WHERE slug = 'clothing'), 'published'),
('Go Programming Language', 'go-programming-language', 'The Go Programming Language by Donovan and Kernighan', 3499, 'USD', 'SEED-BOOK-001', (SELECT id FROM categories WHERE slug = 'books'), 'published'),
('Clean Code', 'clean-code', 'A Handbook of Agile Software Craftsmanship', 2999, 'USD', 'SEED-BOOK-002', (SELECT id FROM categories WHERE slug = 'books'), 'published'),
('Desk Lamp', 'desk-lamp', 'Adjustable LED desk lamp with USB charging', 2499, 'USD', 'SEED-HOME-001', (SELECT id FROM categories WHERE slug = 'home-garden'), 'published'),
('Plant Pot Set', 'plant-pot-set', 'Set of 3 ceramic plant pots', 1999, 'USD', 'SEED-HOME-002', (SELECT id FROM categories WHERE slug = 'home-garden'), 'published'),
('Yoga Mat', 'yoga-mat', 'Non-slip yoga mat with carrying strap', 2499, 'USD', 'SEED-SPORT-001', (SELECT id FROM categories WHERE slug = 'sports'), 'published'),
('Water Bottle', 'water-bottle', 'Insulated stainless steel water bottle 750ml', 1499, 'USD', 'SEED-SPORT-002', (SELECT id FROM categories WHERE slug = 'sports'), 'published')
ON CONFLICT (slug) DO NOTHING;

-- Inventory levels (inventory owns stock; products no longer carries it)
INSERT INTO inventory_levels (product_id, available_stock, reserved_stock) VALUES
((SELECT id FROM products WHERE slug = 'wireless-headphones'), 100, 0),
((SELECT id FROM products WHERE slug = 'bluetooth-speaker'), 50, 0),
((SELECT id FROM products WHERE slug = 'usb-c-hub'), 75, 0),
((SELECT id FROM products WHERE slug = 'cotton-tshirt'), 200, 0),
((SELECT id FROM products WHERE slug = 'denim-jeans'), 150, 0),
((SELECT id FROM products WHERE slug = 'running-shoes'), 80, 0),
((SELECT id FROM products WHERE slug = 'go-programming-language'), 60, 0),
((SELECT id FROM products WHERE slug = 'clean-code'), 45, 0),
((SELECT id FROM products WHERE slug = 'desk-lamp'), 90, 0),
((SELECT id FROM products WHERE slug = 'plant-pot-set'), 120, 0),
((SELECT id FROM products WHERE slug = 'yoga-mat'), 100, 0),
((SELECT id FROM products WHERE slug = 'water-bottle'), 200, 0)
ON CONFLICT (product_id) DO NOTHING;

-- Sample promotions
INSERT INTO promotions (code, type, value, min_order_amount, max_discount, max_uses, starts_at, expires_at, active) VALUES
('WELCOME10', 'percentage', 10, 1000, 5000, 1000, NOW(), NOW() + INTERVAL '1 year', true),
('SAVE20', 'percentage', 20, 5000, 10000, 500, NOW(), NOW() + INTERVAL '6 months', true),
('FLAT500', 'fixed_amount', 500, 2000, NULL, 200, NOW(), NOW() + INTERVAL '3 months', true)
ON CONFLICT (code) DO NOTHING;
