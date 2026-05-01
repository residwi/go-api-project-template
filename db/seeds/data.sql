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
INSERT INTO products (name, slug, description, price, currency, sku, category_id, status, stock_quantity) VALUES
('Wireless Headphones', 'wireless-headphones', 'Premium wireless headphones with noise cancellation', 9999, 'USD', 'SEED-ELEC-001', (SELECT id FROM categories WHERE slug = 'electronics'), 'published', 100),
('Bluetooth Speaker', 'bluetooth-speaker', 'Portable bluetooth speaker with deep bass', 4999, 'USD', 'SEED-ELEC-002', (SELECT id FROM categories WHERE slug = 'electronics'), 'published', 50),
('USB-C Hub', 'usb-c-hub', '7-in-1 USB-C hub with HDMI and ethernet', 3499, 'USD', 'SEED-ELEC-003', (SELECT id FROM categories WHERE slug = 'electronics'), 'published', 75),
('Cotton T-Shirt', 'cotton-tshirt', 'Comfortable 100% cotton t-shirt', 1999, 'USD', 'SEED-CLOTH-001', (SELECT id FROM categories WHERE slug = 'clothing'), 'published', 200),
('Denim Jeans', 'denim-jeans', 'Classic fit denim jeans', 4999, 'USD', 'SEED-CLOTH-002', (SELECT id FROM categories WHERE slug = 'clothing'), 'published', 150),
('Running Shoes', 'running-shoes', 'Lightweight running shoes with cushioned sole', 7999, 'USD', 'SEED-CLOTH-003', (SELECT id FROM categories WHERE slug = 'clothing'), 'published', 80),
('Go Programming Language', 'go-programming-language', 'The Go Programming Language by Donovan and Kernighan', 3499, 'USD', 'SEED-BOOK-001', (SELECT id FROM categories WHERE slug = 'books'), 'published', 60),
('Clean Code', 'clean-code', 'A Handbook of Agile Software Craftsmanship', 2999, 'USD', 'SEED-BOOK-002', (SELECT id FROM categories WHERE slug = 'books'), 'published', 45),
('Desk Lamp', 'desk-lamp', 'Adjustable LED desk lamp with USB charging', 2499, 'USD', 'SEED-HOME-001', (SELECT id FROM categories WHERE slug = 'home-garden'), 'published', 90),
('Plant Pot Set', 'plant-pot-set', 'Set of 3 ceramic plant pots', 1999, 'USD', 'SEED-HOME-002', (SELECT id FROM categories WHERE slug = 'home-garden'), 'published', 120),
('Yoga Mat', 'yoga-mat', 'Non-slip yoga mat with carrying strap', 2499, 'USD', 'SEED-SPORT-001', (SELECT id FROM categories WHERE slug = 'sports'), 'published', 100),
('Water Bottle', 'water-bottle', 'Insulated stainless steel water bottle 750ml', 1499, 'USD', 'SEED-SPORT-002', (SELECT id FROM categories WHERE slug = 'sports'), 'published', 200)
ON CONFLICT (slug) DO NOTHING;

-- Sample promotions
INSERT INTO promotions (code, type, value, min_order_amount, max_discount, max_uses, starts_at, expires_at, active) VALUES
('WELCOME10', 'percentage', 10, 1000, 5000, 1000, NOW(), NOW() + INTERVAL '1 year', true),
('SAVE20', 'percentage', 20, 5000, 10000, 500, NOW(), NOW() + INTERVAL '6 months', true),
('FLAT500', 'fixed_amount', 500, 2000, NULL, 200, NOW(), NOW() + INTERVAL '3 months', true)
ON CONFLICT (code) DO NOTHING;
