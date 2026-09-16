package vault

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// parseUUID parses s as a UUID, wrapping any error with the given field
// name so callers get a useful message about which column failed to parse
// (e.g. "parsing catalog id: invalid UUID length").
func parseUUID(s, field string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("parsing %s: %w", field, err)
	}
	return id, nil
}

// buildInClause returns "?, ?, ?" for len(ids) placeholders and the
// corresponding []any args (as strings, since UUIDs are stored as TEXT).
func buildInClause(ids []uuid.UUID) (string, []any) {
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id.String()
	}
	return strings.Join(placeholders, ", "), args
}

// dedupeUUIDs returns ids with duplicates removed, preserving first-seen
// order.
func dedupeUUIDs(ids []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]bool, len(ids))
	unique := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}
	return unique
}

// nullableUUIDString returns id.String(), or nil (a SQL NULL) when id is
// nil — for writing an optional *uuid.UUID column.
func nullableUUIDString(id *uuid.UUID) any {
	if id == nil {
		return nil
	}
	return id.String()
}

// orEmpty returns s unchanged unless it's nil, in which case it returns a
// non-nil empty slice. A nil slice and a zero-length slice are the same
// slice to Go, but not to encoding/json: nil marshals as "null", not "[]".
// Used at the few spots that produce a slice by map lookup or early return
// rather than by appending — a range/append naturally ends up non-nil even
// at zero length, so callers that already build a slice that way don't need
// this.
func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
