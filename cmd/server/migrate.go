package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/hiidz/uno/internal/provider"
	"github.com/hiidz/uno/internal/vault"
)

// runMigrate is the migrate command. The server migrates its database on
// startup, so the command only rehearses: `migrate --dry-run --db <path>`
// prints what migrating the database at path would do, and never writes to
// it. It needs no configuration.
func runMigrate(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("migrate", flag.ContinueOnError)
	dryRun := flags.Bool("dry-run", false, "report what migrating would do, without writing to the database")
	path := flags.String("db", "", "path to the SQLite database")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if !*dryRun || *path == "" {
		return errors.New("usage: uno migrate --dry-run --db <path> (the server migrates on startup; this command only rehearses)")
	}
	return rehearse(*path, stdout)
}

// rehearse dry-runs the migrations on the database at path, checking every
// catalog's recipe with provider.SameRecipe, and prints the report. A
// migration that fails still gets the report of what ran before it, printed
// ahead of the error.
func rehearse(path string, stdout io.Writer) error {
	report, err := vault.DryRun(context.Background(), path, provider.SameRecipe)
	if report.RowsAfter != nil {
		if _, writeErr := io.WriteString(stdout, reportText(path, report)); writeErr != nil {
			return writeErr
		}
	}
	if err != nil {
		return fmt.Errorf("dry run of %s: %w", path, err)
	}
	return nil
}

// reportText is a dry run's report: the schema versions, each migration with
// its notes, the recipe check, then every table's row count before and
// after.
func reportText(path string, r vault.MigrationReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Dry run of %s. Nothing was written to it.\n", path)
	fmt.Fprintf(&b, "Schema version %d -> %d\n", r.From, r.To)
	for _, m := range r.Applied {
		fmt.Fprintf(&b, "\nMigration %d (%s)\n", m.Version, m.Name)
		for _, note := range m.Notes {
			fmt.Fprintf(&b, "  %s\n", note)
		}
	}
	writeRecipeCheck(&b, r)
	tables := maps.Clone(r.RowsBefore)
	maps.Copy(tables, r.RowsAfter)
	fmt.Fprintf(&b, "\n%-24s %8s %8s\n", "Rows", "before", "after")
	for _, table := range slices.Sorted(maps.Keys(tables)) {
		fmt.Fprintf(&b, "%-24s %8s %8s\n", table, countCell(r.RowsBefore, table), countCell(r.RowsAfter, table))
	}
	return b.String()
}

// writeRecipeCheck writes how many catalogs' params the dry run compared
// before and after migrating and every mismatch, or nothing when it
// compared none.
func writeRecipeCheck(b *strings.Builder, r vault.MigrationReport) {
	if r.RecipesChecked == 0 {
		return
	}
	fmt.Fprintf(b, "\nRecipe check: %d catalogs, %d whose params before and after migrating would fetch different titles\n",
		r.RecipesChecked, len(r.RecipeMismatches))
	for _, mismatch := range r.RecipeMismatches {
		fmt.Fprintf(b, "  %s\n", mismatch)
	}
}

// countCell is table's row count in counts, or "-" where the table doesn't
// exist.
func countCell(counts map[string]int, table string) string {
	if n, ok := counts[table]; ok {
		return strconv.Itoa(n)
	}
	return "-"
}
