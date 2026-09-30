package vault

import (
	"encoding/json"
	"fmt"
	"time"

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

// idsJSON is ids as one JSON array of strings, the single argument an id
// list binds as: every query over a list of ids reads it with
// `IN (SELECT value FROM json_each(?))`, so a list of any length is one
// parameter.
func idsJSON(ids []uuid.UUID) string {
	strs := make([]string, len(ids))
	for i, id := range ids {
		strs[i] = id.String()
	}
	b, _ := json.Marshal(strs) // a []string always encodes
	return string(b)
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

// nullableString returns s, or nil (a SQL NULL) when s is empty — for
// writing an optional TEXT column such as sub_key.
func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// utcTimestamp is t in the RFC3339 UTC form every stored timestamp takes,
// so the two compare as strings.
func utcTimestamp(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}
