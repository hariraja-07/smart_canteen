package main

import "database/sql"

// MenuItem is a dish as it is stored. The wire form is menuItemResponse in
// dto.go; this type is only ever scanned from a row.
type MenuItem struct {
	ID          int64
	Name        string
	Category    string
	Price       int
	Description string
	Available   bool
}

// listMenuItems returns the whole menu, sold out items included. They are listed
// rather than hidden so the client can show the dish greyed out; a menu that
// silently drops what you cannot buy reads as a bug.
//
// The price is cast to INT in SQL rather than read as the NUMERIC(10,2) column
// or as a float8. menu_items_whole_price guarantees the value is integral, so
// the cast is exact, and it means the type is an int from the database all the
// way to the response instead of passing through a float to get there.
func listMenuItems(db *sql.DB) ([]MenuItem, error) {
	rows, err := db.Query(`
SELECT id, name, category, price::INT, description, available
FROM menu_items
ORDER BY category, name`)
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
