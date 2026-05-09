package storage

import "fmt"

// temporallyEffective returns a SQL fragment matching rows that are currently
// effective per the bitemporal validity columns (valid_from, valid_to).
// Composes with deleted_at IS NULL and any other filters via AND.
//
// alias is the SQL alias the surrounding query uses for the table being filtered
// (e.g. "a" for assets, "l" for locations, "i" for tags).
//
// NULL valid_from is treated as "always-was" and NULL valid_to as "open-ended"
// so rows with unset windows remain visible by default.
func temporallyEffective(alias string) string {
	return fmt.Sprintf(
		"(%[1]s.valid_from IS NULL OR %[1]s.valid_from <= NOW()) AND (%[1]s.valid_to IS NULL OR %[1]s.valid_to > NOW())",
		alias,
	)
}
