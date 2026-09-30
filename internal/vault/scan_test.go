package vault

import (
	"context"
	"strings"
	"testing"
)

// queryStrings and queryUUIDs name what they were reading in every error: a
// query that fails, a row that doesn't scan as text, and a value that isn't
// a UUID.
func TestQueryStringsReportsErrors(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	for query, want := range map[string]string{
		`SELECT nope FROM nowhere`:  "querying thing list",
		`SELECT NULL`:               "scanning thing",
		`SELECT 'not a uuid' AS id`: "parsing thing",
	} {
		_, err := queryUUIDs(ctx, db.conn, "thing", query)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("queryUUIDs(%s) = %v, want an error containing %q", query, err, want)
		}
	}
	got, err := queryStrings(ctx, db.conn, "thing", `SELECT 'a' UNION ALL SELECT 'b'`)
	if err != nil || strings.Join(got, ",") != "a,b" {
		t.Errorf("queryStrings = %q, %v; want a,b", got, err)
	}
}
