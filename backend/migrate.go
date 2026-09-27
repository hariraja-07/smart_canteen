package main

import (
	"database/sql"
	"fmt"
	"log"
)

// migration is one numbered, ordered schema change. Versions must never be
// reused or renumbered: an applied version is recorded in schema_migrations and
// skipped forever after, so changing a migration's body only affects databases
// that have not run it yet.
type migration struct {
	version int
	name    string
	run     func(*sql.Tx) error
}

func migrations() []migration {
	return []migration{
		{1, "initial schema", migrateInitial},
		{2, "indian canteen menu and whole-rupee prices", migrateMenu},
		{3, "user coin balance and role constraint", migrateUserCoins},
		{4, "coin ledger", migrateLedger},
		{5, "whole coin price constraint", migratePriceConstraint},
		{6, "student role default", migrateRoleDefault},
		{7, "orders", migrateOrders},
	}
}

// migrateRoleDefault fixes a hole left by migration 3, which replaced the
// 'customer' role with the four-role set and constrained the column but left the
// column default as 'customer'. Any INSERT that omitted role therefore produced
// a value the new constraint rejects. Nothing in the app omits role, so this sat
// unnoticed, but the schema should not contain a default that cannot be used.
func migrateRoleDefault(tx *sql.Tx) error {
	_, err := tx.Exec(`ALTER TABLE users ALTER COLUMN role SET DEFAULT 'student'`)
	return err
}

// migrateOrders upgrades the orders scaffolded by migration 1 to the whole-coin
// economy. The tables already existed with decimal NUMERIC money columns, so
// this alters them in place rather than replacing them, and it works whether or
// not any orders have been placed.
//
// There is no canteen_id: the canteen is a single deployment-wide entity, the way
// the students share one canteen. Its coins are tracked through the
// canteen_management user account, which the ledger already knows how to credit.
func migrateOrders(tx *sql.Tx) error {
	_, err := tx.Exec(`
-- Refuse to convert if anything is fractional rather than silently rounding a
-- real total. Nothing has ever written a fractional total, so this is a
-- tripwire against a future regression, not a migration step.
DO $$
BEGIN
	IF EXISTS (SELECT 1 FROM orders WHERE total <> FLOOR(total)) THEN
		RAISE EXCEPTION 'orders.total holds a fractional value; whole coins require manual review';
	END IF;
END $$;

ALTER TABLE orders ALTER COLUMN total TYPE INT USING (total)::INT;
ALTER TABLE orders ALTER COLUMN total SET NOT NULL;
ALTER TABLE orders ADD CONSTRAINT orders_total_positive CHECK (total > 0);
ALTER TABLE orders ADD CONSTRAINT orders_status_valid
	CHECK (status IN ('pending', 'preparing', 'ready', 'completed', 'cancelled'));
ALTER TABLE orders ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

-- A student reads their own recent orders, and the canteen works through the
-- queue by status, so both access patterns get an index.
CREATE INDEX orders_user_created_idx ON orders (user_id, created_at DESC);
CREATE INDEX orders_status_created_idx ON orders (status, created_at);

-- order_items snapshots the name and line total. Reading them from menu_items at
-- display time would let a rename or a price change rewrite what someone paid.
ALTER TABLE order_items ADD COLUMN name TEXT NOT NULL DEFAULT '';
UPDATE order_items oi SET name = mi.name
FROM menu_items mi WHERE mi.id = oi.menu_item_id AND oi.name = '';
ALTER TABLE order_items ALTER COLUMN name DROP DEFAULT;

ALTER TABLE order_items ADD COLUMN line_total INT NOT NULL DEFAULT 0;
UPDATE order_items SET line_total = (unit_price)::INT * qty WHERE line_total = 0;
ALTER TABLE order_items ALTER COLUMN line_total DROP DEFAULT;
ALTER TABLE order_items ALTER COLUMN unit_price TYPE INT USING (unit_price)::INT;

ALTER TABLE order_items ADD CONSTRAINT order_items_qty_positive CHECK (qty > 0);
ALTER TABLE order_items ADD CONSTRAINT order_items_price_nonneg CHECK (unit_price >= 0);
ALTER TABLE order_items ADD CONSTRAINT order_items_line_total_ok
	CHECK (line_total = unit_price * qty);
`)
	return err
}

func migrateInitial(tx *sql.Tx) error {
	_, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS users (
	id BIGSERIAL PRIMARY KEY,
	name TEXT NOT NULL,
	email TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	role TEXT NOT NULL DEFAULT 'customer',
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS menu_items (
	id BIGSERIAL PRIMARY KEY,
	name TEXT NOT NULL,
	category TEXT NOT NULL,
	price NUMERIC(10,2) NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	available BOOLEAN NOT NULL DEFAULT true,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS orders (
	id BIGSERIAL PRIMARY KEY,
	user_id BIGINT NOT NULL REFERENCES users(id),
	status TEXT NOT NULL DEFAULT 'pending',
	total NUMERIC(10,2) NOT NULL DEFAULT 0,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS order_items (
	order_id BIGINT NOT NULL REFERENCES orders(id),
	menu_item_id BIGINT NOT NULL REFERENCES menu_items(id),
	qty INT NOT NULL,
	unit_price NUMERIC(10,2) NOT NULL,
	PRIMARY KEY (order_id, menu_item_id)
)`)
	return err
}

func migrateMenu(tx *sql.Tx) error {
	// Round any pre-existing price to a whole rupee. Coins are indivisible, so a
	// fractional price could never be paid exactly. v5 adds the CHECK that keeps
	// this true; doing it here first means that constraint always validates.
	if _, err := tx.Exec(`UPDATE menu_items SET price = ROUND(price)`); err != nil {
		return err
	}

	// Replace the old western dev menu with the canteen menu, but only while
	// nothing references the old rows. Once real orders exist the old items stay
	// put and the canteen is edited through the admin API instead.
	var referenced bool
	if err := tx.QueryRow(`
SELECT EXISTS (
	SELECT 1 FROM order_items oi
	JOIN menu_items m ON m.id = oi.menu_item_id
	WHERE m.name IN ('Pizza Margherita','Paneer Butter Masala','Chicken Biryani',
	                 'Veg Fried Rice','Garlic Naan','Coca-Cola')
)`).Scan(&referenced); err != nil {
		return err
	}
	if referenced {
		return nil
	}
	if _, err := tx.Exec(`
DELETE FROM menu_items WHERE name IN
	('Pizza Margherita','Paneer Butter Masala','Chicken Biryani',
	 'Veg Fried Rice','Garlic Naan','Coca-Cola')`); err != nil {
		return err
	}
	_, err := tx.Exec(`
INSERT INTO menu_items (name, category, price, description, available) VALUES
	('Masala Chai',         'Chai & Snacks',   10, 'Ginger, cardamom, milk', true),
	('Samosa (2 pcs)',      'Chai & Snacks',   20, 'Crisp pastry with spiced potato', true),
	('Veg Puff',            'Chai & Snacks',   25, 'Flaky puff with vegetable filling', true),
	('Buttermilk',          'Chai & Snacks',   15, 'Chilled chaas', true),
	('French Fries',        'Chai & Snacks',   40, 'Salted, fried potato fingers', true),
	('Masala Dosa',         'Breakfast',       50, 'Crisp dosa with potato masala', true),
	('Idli Sambar (2 pcs)', 'Breakfast',       30, 'Steamed idli with sambar', true),
	('Poha',                'Breakfast',       30, 'Flattened rice with onion and spices', true),
	('Upma',                'Breakfast',       30, 'Semolina porridge with vegetables', true),
	('Paratha (2 pcs)',     'Breakfast',       40, 'Layered flatbread with potato', true),
	('Paneer Butter Masala','Main Course',    100, 'Cottage cheese in tomato gravy', true),
	('Chole Bhature',       'Main Course',     80, 'Chickpea curry with fried bread', true),
	('Dal Makhani',         'Main Course',     90, 'Slow-cooked black lentils', true),
	('Mixed Veg Curry',     'Main Course',     70, 'Seasonal vegetables in gravy', true),
	('Chicken Biryani',     'Rice & Biryani', 130, 'Fragrant rice with chicken', true),
	('Veg Biryani',         'Rice & Biryani', 110, 'Fragrant rice with vegetables', true),
	('Veg Fried Rice',      'Rice & Biryani',  70, 'Wok-fried vegetables and rice', true),
	('Garlic Naan',         'Bread',           30, 'Oven-baked naan with garlic', true),
	('Butter Roti (2 pcs)', 'Bread',           20, 'Tandoor-baked buttered bread', true),
	('Tandoori Roti',       'Bread',           25, 'Oven-baked whole wheat roti', true),
	('Coca-Cola (330ml)',   'Drinks',          20, 'Chilled cola', true),
	('Fresh Lime Soda',     'Drinks',          30, 'Sweet or salted lime', true),
	('Mineral Water (1L)',  'Drinks',          20, 'Packaged drinking water', true)`)
	return err
}

func migrateUserCoins(tx *sql.Tx) error {
	// ALTER, not CREATE: the users table already exists, and
	// CREATE TABLE IF NOT EXISTS would silently skip it.
	if _, err := tx.Exec(
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS coin_balance INT NOT NULL DEFAULT 0`,
	); err != nil {
		return err
	}
	// The default role name changes from the old 'customer' to 'student'.
	if _, err := tx.Exec(`UPDATE users SET role = 'student' WHERE role = 'customer'`); err != nil {
		return err
	}
	if _, err := tx.Exec(`ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check`); err != nil {
		return err
	}
	if _, err := tx.Exec(`
ALTER TABLE users ADD CONSTRAINT users_role_check
	CHECK (role IN ('admin','student','staff','canteen_management'))`); err != nil {
		return err
	}
	// Makes an overdraft impossible at the database level, not just in Go.
	_, err := tx.Exec(`ALTER TABLE users ADD CONSTRAINT users_coin_nonneg CHECK (coin_balance >= 0)`)
	return err
}

func migrateLedger(tx *sql.Tx) error {
	_, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS coin_transactions (
	id BIGSERIAL PRIMARY KEY,
	group_id UUID NOT NULL DEFAULT gen_random_uuid(),
	user_id BIGINT NOT NULL REFERENCES users(id),
	amount INT NOT NULL CHECK (amount <> 0),
	kind TEXT NOT NULL CHECK (kind IN
		('exchange_in','order_payment','canteen_revenue','refund')),
	reason TEXT NOT NULL DEFAULT '',
	actor_id BIGINT REFERENCES users(id),
	order_id BIGINT REFERENCES orders(id),
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_coin_tx_user ON coin_transactions(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_coin_tx_group ON coin_transactions(group_id);
CREATE INDEX IF NOT EXISTS idx_coin_tx_kind ON coin_transactions(kind, created_at DESC)`)
	return err
}

func migratePriceConstraint(tx *sql.Tx) error {
	_, err := tx.Exec(`ALTER TABLE menu_items ADD CONSTRAINT menu_items_whole_price CHECK (price = FLOOR(price))`)
	return err
}

// runMigrations applies every migration that has not been recorded yet, in
// version order, each in its own transaction. A migration that fails is rolled
// back and recorded as not applied, so a retry starts from a known state.
func runMigrations(db *sql.DB) error {
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS schema_migrations (
	version    INT PRIMARY KEY,
	name       TEXT NOT NULL,
	applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	for _, m := range migrations() {
		applied, err := migrationApplied(db, m.version)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		if err := applyMigration(db, m); err != nil {
			return fmt.Errorf("migration %d (%s): %w", m.version, m.name, err)
		}
		log.Printf("applied migration %d: %s", m.version, m.name)
	}
	return nil
}

func migrationApplied(db *sql.DB, version int) (bool, error) {
	var applied bool
	err := db.QueryRow(
		`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, version,
	).Scan(&applied)
	if err != nil {
		return false, fmt.Errorf("check migration %d: %w", version, err)
	}
	return applied, nil
}

func applyMigration(db *sql.DB, m migration) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := m.run(tx); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`, m.version, m.name,
	); err != nil {
		return err
	}
	return tx.Commit()
}
