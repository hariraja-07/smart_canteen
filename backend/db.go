package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type MenuItem struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Category    string  `json:"category"`
	Price       float64 `json:"price"`
	Description string  `json:"description"`
	Available   bool    `json:"available"`
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func openDB() (*sql.DB, error) {
	dsn := getEnv("DATABASE_URL", "postgres://smartcanteen:smartcanteen_dev@localhost:5432/smart_canteen")
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func migrate(db *sql.DB) error {
	const schema = `
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
);`
	_, err := db.Exec(schema)
	return err
}

func seedMenu(db *sql.DB) error {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM menu_items").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err := db.Exec(`
INSERT INTO menu_items (name, category, price, description, available) VALUES
('Pizza Margherita', 'Main', 9.99, 'Tomato, mozzarella, basil', true),
('Paneer Butter Masala', 'Main', 8.99, 'Cottage cheese in tomato gravy', true),
('Chicken Biryani', 'Main', 11.99, 'Fragrant rice with chicken', true),
('Veg Fried Rice', 'Rice', 7.49, 'Wok-fried vegetables and rice', true),
('Garlic Naan', 'Bread', 2.49, 'Oven-baked naan with garlic', true),
('Coca-Cola', 'Drinks', 1.99, 'Classic cola 330ml', true)`)
	return err
}

func listMenuItems(db *sql.DB) ([]MenuItem, error) {
	rows, err := db.Query(`SELECT id, name, category, price::float8, description, available FROM menu_items ORDER BY category, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []MenuItem{}
	for rows.Next() {
		var it MenuItem
		if err := rows.Scan(&it.ID, &it.Name, &it.Category, &it.Price, &it.Description, &it.Available); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

func initDB() (*sql.DB, error) {
	db, err := openDB()
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	if err := seedMenu(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("seed menu: %w", err)
	}
	return db, nil
}