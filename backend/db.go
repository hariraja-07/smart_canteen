package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// openDB is the single way this program obtains a usable database: it connects,
// brings the schema up to date, and seeds the demo users. Every caller goes
// through here, so none of them can end up running against a schema that was
// never migrated, which is the failure the previous pair of functions made
// possible.
func openDB() (*sql.DB, error) {
	db, err := connectDB()
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}
	if err := runMigrations(db); err != nil {
		db.Close()
		return nil, err
	}
	if err := seedDemoUsers(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// connectDB dials Postgres and confirms the connection is live. Migrations and
// seeding are openDB's job, so a caller that only wants a connection cannot
// quietly get one against an unmigrated schema.
func connectDB() (*sql.DB, error) {
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
