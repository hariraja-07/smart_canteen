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
	dsn := getEnv("DATABASE_URL", "")
	if dsn == "" {
		return nil, fmt.Errorf("DATABASE_URL is required (see backend/.env.example)")
	}
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
	if err := runMigrations(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
