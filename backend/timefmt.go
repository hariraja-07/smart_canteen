package main

import (
	"fmt"
	"time"
)

// formatTimestamp renders a timestamp column as RFC3339 for JSON responses.
func formatTimestamp(v any) string {
	switch t := v.(type) {
	case time.Time:
		return t.UTC().Format(time.RFC3339)
	default:
		return fmt.Sprint(v)
	}
}
