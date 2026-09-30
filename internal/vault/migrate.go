package vault

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/hiidz/uno/internal/vault/migrations"
)

// AppliedMigration is one migration a run applied, with the notes it
// returned.
type AppliedMigration struct {
	Version int
	Name    string
	Notes   []string
}

// MigrationReport is what DryRun found: the schema version before and after,
// the migrations applied between them, and each table's row count before and
// after, keyed by table name. When the copy's catalogs still had their own
// params before migrating, RecipesChecked is how many catalogs the recipe
// check compared before and after, and RecipeMismatches names each one it
// found different, with the difference. When the migrations created the
// publications, PublicationsChecked is how many the publication check ran
// today's validators over, and PublicationProblems names each one they
// refuse, with the reason.
type MigrationReport struct {
	From, To            int
	Applied             []AppliedMigration
	RowsBefore          map[string]int
	RowsAfter           map[string]int
	RecipesChecked      int
	RecipeMismatches    []string
	PublicationsChecked int
	PublicationProblems []string
	// PushHashesBackfilled and PushHashesPending count the collections the
	// pushed_hash migration gave a pushed hash and left pending, and
	// PushHashMismatches names each backfilled one whose hash the vault's live
	// push payload doesn't give: none, unless the migration's own copy of the
	// payload has drifted from the vault's.
	PushHashesBackfilled int
	PushHashesPending    int
	PushHashMismatches   []string
}

// RecipeCheck reports how before and after, one catalog's params before and
// after migrating, would fetch different titles as a recipe of catalogType
// and catalogProvider, or returns nil when they fetch the same ones. It is live code, which a
// migration can't run, so DryRun takes it from its caller:
// provider.SameRecipe, which this package can't import.
type RecipeCheck func(catalogType, catalogProvider, before, after string) error

// migrate brings the database at path up to the last version in list, on a
// connection of its own with foreign keys off, before InitDB opens the pool.
// With migrations pending it first writes a backup beside path, then runs
// each migration in its own transaction. A migration that fails, or leaves a
// foreign key broken, rolls back, and the ones before it stay applied.
func migrate(ctx context.Context, path string, list []migrations.Migration) error {
	d, err := openMigrationDB(path, "")
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	return migrateOpen(ctx, d, path, list)
}

// migrateOpen is migrate over the connection d has open to path.
func migrateOpen(ctx context.Context, d *sql.DB, path string, list []migrations.Migration) error {
	_, pending, err := pendingMigrations(ctx, d, list)
	if err != nil || len(pending) == 0 {
		return err
	}
	backup, err := backUp(ctx, d, path, list[len(list)-1].Version)
	if err != nil {
		return err
	}
	applied, err := applyMigrations(ctx, d, pending)
	logMigrated(backup, applied)
	return err
}

// openMigrationDB opens the database at path for migrating. It sets no
// foreign_keys pragma, so foreign keys stay off, as a table rebuild needs;
// the runner checks them itself before each commit. Its transactions begin
// IMMEDIATE, taking the write lock before they read the schema version, so
// two processes starting together can't both apply one migration. query is
// appended to the DSN's own parameters.
func openMigrationDB(path, query string) (*sql.DB, error) {
	d, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_txlock=immediate"+query)
	if err != nil {
		return nil, fmt.Errorf("opening the database to migrate: %w", err)
	}
	return d, nil
}

// schemaVersion reads the user_version q sees.
func schemaVersion(ctx context.Context, q queryRower) (int, error) {
	var version int
	if err := q.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return 0, fmt.Errorf("reading the schema version: %w", err)
	}
	return version, nil
}

// pendingMigrations reads the schema version d is at and returns it with the
// migrations in list still to run. A version outside 0 to len(list) is an
// error: this build can't know that schema.
func pendingMigrations(ctx context.Context, d *sql.DB, list []migrations.Migration) (int, []migrations.Migration, error) {
	version, err := schemaVersion(ctx, d)
	if err != nil {
		return 0, nil, err
	}
	if version < 0 || version > len(list) {
		return version, nil, fmt.Errorf("the database is at schema version %d, which this build (versions 0 to %d) cannot read: run the build that wrote it, or restore the backup taken before the upgrade", version, len(list))
	}
	return version, list[version:], nil
}

// backUp copies the database to <path>.pre-v<version>-<UTC time>.bak, where
// version is the one about to be migrated to, and returns the copy's path.
// VACUUM INTO writes the same bytes for the same content, so a start that
// fails the same way as the one before finds its copy identical to the
// newest earlier backup for version; the new copy is then dropped and that
// backup's path returned, and a restart loop keeps one copy, not one each.
func backUp(ctx context.Context, d *sql.DB, path string, version int) (string, error) {
	// A path Glob can't read as a pattern has no earlier backups to compare.
	earlier, _ := filepath.Glob(fmt.Sprintf("%s.pre-v%d-*.bak", path, version))
	backup := fmt.Sprintf("%s.pre-v%d-%s.bak", path, version, time.Now().UTC().Format("20060102T150405.000Z"))
	if _, err := d.ExecContext(ctx, `VACUUM INTO ?`, backup); err != nil {
		return "", fmt.Errorf("backing up the database before migrating: %w", err)
	}
	return dropRepeatBackup(backup, earlier), nil
}

// dropRepeatBackup removes backup and returns the newest of earlier instead
// when the two hold the same bytes; otherwise it returns backup. Backup names
// sort by the time they were written.
func dropRepeatBackup(backup string, earlier []string) string {
	if len(earlier) == 0 {
		return backup
	}
	newest := slices.Max(earlier)
	if sameBytes(newest, backup) && os.Remove(backup) == nil {
		return newest
	}
	return backup
}

// sameBytes reports whether files a and b hold the same bytes. A file that
// can't be read matches nothing.
func sameBytes(a, b string) bool {
	hashA, errA := fileHash(a)
	hashB, errB := fileHash(b)
	return errA == nil && errB == nil && hashA == hashB
}

// fileHash is the hex sha256 of the file at path, read as a stream.
func fileHash(path string) (string, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// applyMigrations runs each of pending in order, stopping at the first that
// fails, and returns the ones that were applied.
func applyMigrations(ctx context.Context, d *sql.DB, pending []migrations.Migration) ([]AppliedMigration, error) {
	applied := make([]AppliedMigration, 0, len(pending))
	for _, m := range pending {
		notes, err := applyMigration(ctx, d, m)
		if err != nil {
			return applied, fmt.Errorf("migration %d (%s): %w", m.Version, m.Name, err)
		}
		applied = append(applied, AppliedMigration{Version: m.Version, Name: m.Name, Notes: notes})
	}
	return applied, nil
}

// applyMigration runs m in one transaction that also sets user_version to
// m.Version, and commits it only if every foreign key still holds.
func applyMigration(ctx context.Context, d *sql.DB, m migrations.Migration) ([]string, error) {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("starting transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op once Commit succeeds

	if err := requireVersion(ctx, tx, m.Version-1); err != nil {
		return nil, err
	}
	notes, err := m.Up(ctx, tx)
	if err != nil {
		return nil, err
	}
	return notes, commitVersion(ctx, tx, m.Version)
}

// requireVersion fails unless tx, holding the write lock, sees the database
// at schema version want. Any other version means another process migrated
// it after pendingMigrations looked.
func requireVersion(ctx context.Context, tx *sql.Tx, want int) error {
	version, err := schemaVersion(ctx, tx)
	if err != nil {
		return err
	}
	if version != want {
		return fmt.Errorf("the database is at schema version %d, not %d: another process migrated it meanwhile; start again", version, want)
	}
	return nil
}

// commitVersion checks foreign keys, sets user_version to version and
// commits tx.
func commitVersion(ctx context.Context, tx *sql.Tx, version int) error {
	if err := checkForeignKeys(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
		return fmt.Errorf("setting the schema version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing: %w", err)
	}
	return nil
}

// checkForeignKeys fails if foreign_key_check reports any row, naming the
// first ten.
func checkForeignKeys(ctx context.Context, tx *sql.Tx) error {
	violations, err := foreignKeyViolations(ctx, tx)
	if err != nil {
		return err
	}
	if len(violations) > 0 {
		return fmt.Errorf("foreign_key_check found %d broken references: %s", len(violations), strings.Join(violations[:min(len(violations), 10)], "; "))
	}
	return nil
}

// foreignKeyViolations lists every row foreign_key_check reports, as
// "<table> row <rowid> → <parent>".
func foreignKeyViolations(ctx context.Context, tx *sql.Tx) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return nil, fmt.Errorf("checking foreign keys: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var violations []string
	for rows.Next() {
		var table, parent string
		var rowid sql.NullInt64
		var fkid int
		if err := rows.Scan(&table, &rowid, &parent, &fkid); err != nil {
			return nil, fmt.Errorf("checking foreign keys: %w", err)
		}
		violations = append(violations, fmt.Sprintf("%s row %d → %s", table, rowid.Int64, parent))
	}
	return violations, rows.Err()
}

// logMigrated logs the backup a startup migration wrote and each migration
// it applied, with its notes.
func logMigrated(backup string, applied []AppliedMigration) {
	log.Printf("Backed up the database to %s", backup)
	for _, m := range applied {
		log.Printf("Migrated the database to schema version %d (%s)", m.Version, m.Name)
		for _, note := range m.Notes {
			log.Printf("  %s", note)
		}
	}
}

// LiveChecks is the live code a dry run checks the migrated copy with, which
// a migration can't run and this package can't import: provider.SameRecipe
// and provider.ValidateRecipe. Both are required.
type LiveChecks struct {
	// SameRecipe compares each catalog's params from before migrating with
	// its recipe's after, while the copy's catalogs hold their own params.
	SameRecipe RecipeCheck
	// ValidRecipe checks every recipe of each publication the migrations
	// create, with no network call.
	ValidRecipe CatalogParamsValidator
}

// DryRun reports what migrating the database at path would do, without
// writing to it: the file is opened read-only and copied to a temporary
// directory, and every pending migration runs on the copy, which is then
// deleted. checks run over the copy; see LiveChecks.
func DryRun(ctx context.Context, path string, checks LiveChecks) (MigrationReport, error) {
	return dryRun(ctx, path, migrations.All(), checks)
}

// dryRun is DryRun over list.
func dryRun(ctx context.Context, path string, list []migrations.Migration, checks LiveChecks) (MigrationReport, error) {
	dir, err := os.MkdirTemp("", "uno-migrate-")
	if err != nil {
		return MigrationReport{}, fmt.Errorf("creating a directory for the copy: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	copyPath := filepath.Join(dir, "vault.db")
	if err := copyReadOnly(ctx, path, copyPath); err != nil {
		return MigrationReport{}, err
	}
	return migrateCopy(ctx, copyPath, list, checks)
}

// copyReadOnly writes a copy of the database at src to dst, opening src
// read-only. A read-only open of a WAL database creates its -wal and -shm
// files when they're missing, so any that weren't there before are removed
// again once src is closed.
func copyReadOnly(ctx context.Context, src, dst string) error {
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("reading the database: %w", err)
	}
	created := absentSidecars(src)
	defer removeFiles(created)

	d, err := openMigrationDB(src, "&mode=ro")
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	if _, err := d.ExecContext(ctx, `VACUUM INTO ?`, dst); err != nil {
		return fmt.Errorf("copying the database: %w", err)
	}
	return nil
}

// absentSidecars returns the -wal and -shm paths beside path that don't
// exist.
func absentSidecars(path string) []string {
	var absent []string
	for _, sidecar := range []string{path + "-wal", path + "-shm"} {
		if _, err := os.Stat(sidecar); errors.Is(err, fs.ErrNotExist) {
			absent = append(absent, sidecar)
		}
	}
	return absent
}

// removeFiles removes each of paths, ignoring any already gone.
func removeFiles(paths []string) {
	for _, path := range paths {
		_ = os.Remove(path)
	}
}

// migrateCopy migrates the throwaway copy at path through list, counting
// every table's rows before and after, checks its recipes with
// checks.SameRecipe (checkRecipes), the publications it creates with
// checks.ValidRecipe (checkPublications), and the push hashes it backfills
// with the live push payload (checkPushHashes).
func migrateCopy(ctx context.Context, path string, list []migrations.Migration, checks LiveChecks) (MigrationReport, error) {
	d, err := openMigrationDB(path, "")
	if err != nil {
		return MigrationReport{}, err
	}
	defer func() { _ = d.Close() }()
	return checkRecipes(ctx, d, checks.SameRecipe, func() (MigrationReport, error) {
		return migrateAndCheck(ctx, d, list, checks.ValidRecipe)
	})
}

// migrateAndCheck is migrateCopyOpen, then checkPublications and
// checkPushHashes over what it migrated, unless it failed.
func migrateAndCheck(ctx context.Context, d *sql.DB, list []migrations.Migration, validRecipe CatalogParamsValidator) (MigrationReport, error) {
	report, err := migrateCopyOpen(ctx, d, list)
	if err != nil {
		return report, err
	}
	if report, err = checkPublications(ctx, d, report, validRecipe); err != nil {
		return report, err
	}
	return checkPushHashes(ctx, d, report)
}

// pushedHashMigration is the name of the migration that backfills
// collections.pushed_hash.
const pushedHashMigration = "pushed_hash"

// checkPushHashes compares the pushed hash the pushed_hash migration stored
// for each collection with the hash of the vault's live push payload for it,
// adding what it found to report. A collection whose two differ would read
// as needing a push once deployed. It checks nothing unless this dry run
// applied that migration.
func checkPushHashes(ctx context.Context, d *sql.DB, report MigrationReport) (MigrationReport, error) {
	if !slices.ContainsFunc(report.Applied, isPushedHashMigration) {
		return report, nil
	}
	collections, err := selectLeanCollections(ctx, d, "col.pushed_hash IS NOT NULL ORDER BY col.id")
	if err != nil {
		return report, err
	}
	trees, err := assembleCollectionTree(ctx, d, collections, leanCatalogsByIDs)
	if err != nil {
		return report, err
	}
	report.PushHashesBackfilled = len(trees)
	report.PushHashMismatches = pushHashMismatches(trees)
	err = d.QueryRowContext(ctx, `SELECT count(*) FROM collections WHERE pushed_hash IS NULL`).Scan(&report.PushHashesPending)
	return report, err
}

// isPushedHashMigration reports whether m is the pushed_hash migration.
func isPushedHashMigration(m AppliedMigration) bool {
	return m.Name == pushedHashMigration
}

// pushHashMismatches names each of trees whose stored pushed hash isn't the
// hash of its live push payload.
func pushHashMismatches(trees []CollectionWithFolders) []string {
	var mismatches []string
	for _, tree := range trees {
		if raw, err := tree.PushJSON(); err != nil || PushHash(raw) != tree.pushedHash {
			mismatches = append(mismatches, fmt.Sprintf("collection %s (%s)", tree.ID, tree.Title))
		}
	}
	return mismatches
}

// checkRecipes reads every catalog's params from d, runs migrate, then
// compares each with its recipe's params after, through sameRecipe, adding
// what it found to migrate's report. It checks nothing when d's catalogs
// hold no params of their own beforehand, a database already past
// migration 2 or an empty one, or when migrate fails.
func checkRecipes(ctx context.Context, d *sql.DB, sameRecipe RecipeCheck, migrate func() (MigrationReport, error)) (MigrationReport, error) {
	before, err := catalogParamsBefore(ctx, d)
	if err != nil {
		return MigrationReport{}, err
	}
	report, err := migrate()
	if err != nil || len(before) == 0 {
		return report, err
	}
	report.RecipesChecked = len(before)
	report.RecipeMismatches, err = recipeMismatches(ctx, d, before, sameRecipe)
	return report, err
}

// paramsBefore is one catalog as it was before migrating: its name, type,
// provider and own params.
type paramsBefore struct {
	id, name, catalogType, provider, params string
}

// catalogParamsBefore reads every catalog's own params from d, in id order,
// or nothing when catalogs has no params column.
func catalogParamsBefore(ctx context.Context, d *sql.DB) ([]paramsBefore, error) {
	var hasParams bool
	err := d.QueryRowContext(ctx, `SELECT count(*) > 0 FROM pragma_table_info('catalogs') WHERE name = 'params'`).Scan(&hasParams)
	if err != nil || !hasParams {
		return nil, err
	}
	return readParamsBefore(ctx, d)
}

// readParamsBefore reads every catalog's own params from d, in id order.
func readParamsBefore(ctx context.Context, d *sql.DB) ([]paramsBefore, error) {
	rows, err := d.QueryContext(ctx, `SELECT id, name, type, provider, params FROM catalogs ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("reading catalogs' params: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var catalogs []paramsBefore
	for rows.Next() {
		var c paramsBefore
		if err := rows.Scan(&c.id, &c.name, &c.catalogType, &c.provider, &c.params); err != nil {
			return nil, fmt.Errorf("reading catalogs' params: %w", err)
		}
		catalogs = append(catalogs, c)
	}
	return catalogs, rows.Err()
}

// recipeMismatches compares each of before with its catalog's recipe params
// in d through sameRecipe, and describes each catalog that differs or has no
// recipe.
func recipeMismatches(ctx context.Context, d *sql.DB, before []paramsBefore, sameRecipe RecipeCheck) ([]string, error) {
	after, err := recipeParamsByCatalog(ctx, d)
	if err != nil {
		return nil, err
	}
	var mismatches []string
	for _, c := range before {
		params, ok := after[c.id]
		if !ok {
			mismatches = append(mismatches, fmt.Sprintf("catalog %s (%s): no recipe after migrating", c.id, c.name))
		} else if err := sameRecipe(c.catalogType, c.provider, c.params, params); err != nil {
			mismatches = append(mismatches, fmt.Sprintf("catalog %s (%s): %v", c.id, c.name, err))
		}
	}
	return mismatches, nil
}

// recipeParamsByCatalog reads each catalog's recipe params from d, keyed by
// catalog id.
func recipeParamsByCatalog(ctx context.Context, d *sql.DB) (map[string]string, error) {
	rows, err := d.QueryContext(ctx, `SELECT c.id, r.params FROM `+catalogsWithRecipes)
	if err != nil {
		return nil, fmt.Errorf("reading recipes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	params := map[string]string{}
	for rows.Next() {
		var id, p string
		if err := rows.Scan(&id, &p); err != nil {
			return nil, fmt.Errorf("reading recipes: %w", err)
		}
		params[id] = p
	}
	return params, rows.Err()
}

// checkPublications runs today's checks over every publication the
// migrations created, adding what it found to report: the form validators a
// subscribe runs, then validRecipe over each recipe, which makes no network
// call. A refused publication is reported, not an error: a migration
// publishes each source as it stood, unchecked. It checks nothing when the
// copy had publications before migrating, or has none after.
func checkPublications(ctx context.Context, d *sql.DB, report MigrationReport, validRecipe CatalogParamsValidator) (MigrationReport, error) {
	_, before := report.RowsBefore["publications"]
	_, after := report.RowsAfter["publications"]
	if before || !after {
		return report, nil
	}
	pubs, err := readStoredPublications(ctx, d)
	for _, pub := range pubs {
		report.PublicationsChecked++
		if problem := checkSnapshot(pub.raw, validRecipe); problem != nil {
			report.PublicationProblems = append(report.PublicationProblems, fmt.Sprintf("%s %s (%s): %v", pub.kind, pub.id, pub.title, problem))
		}
	}
	return report, err
}

// storedPublicationRow is one publication as the publication check reads
// it.
type storedPublicationRow struct{ id, kind, title, raw string }

// readStoredPublications reads every publication in d, by kind, then
// title, then id.
func readStoredPublications(ctx context.Context, d *sql.DB) ([]storedPublicationRow, error) {
	rows, err := d.QueryContext(ctx, `SELECT id, kind, title, snapshot FROM publications ORDER BY kind, title, id`)
	if err != nil {
		return nil, fmt.Errorf("reading publications: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var pubs []storedPublicationRow
	for rows.Next() {
		var p storedPublicationRow
		if err := rows.Scan(&p.id, &p.kind, &p.title, &p.raw); err != nil {
			return nil, fmt.Errorf("reading publications: %w", err)
		}
		pubs = append(pubs, p)
	}
	return pubs, rows.Err()
}

// checkSnapshot runs a publish's checks over the stored snapshot raw, with
// validRecipe in place of the TMDB check.
func checkSnapshot(raw string, validRecipe CatalogParamsValidator) error {
	s, err := decodeSnapshot(raw)
	if err != nil {
		return err
	}
	return publication{snapshot: s}.check(validRecipe)
}

// migrateCopyOpen is migrateCopy over the connection d has open to the copy.
// When a migration fails, the report still covers the ones applied before it,
// with the row counts the copy holds after them, alongside the error.
func migrateCopyOpen(ctx context.Context, d *sql.DB, list []migrations.Migration) (MigrationReport, error) {
	var report MigrationReport
	var err error
	if report.RowsBefore, err = countRows(ctx, d); err != nil {
		return report, err
	}
	var pending []migrations.Migration
	if report.From, pending, err = pendingMigrations(ctx, d, list); err != nil {
		return report, err
	}
	report.Applied, err = applyMigrations(ctx, d, pending)
	report.To = report.From + len(report.Applied)
	var countErr error
	report.RowsAfter, countErr = countRows(ctx, d)
	return report, errors.Join(err, countErr)
}

// countRows counts the rows of every table in d, keyed by table name.
func countRows(ctx context.Context, d *sql.DB) (map[string]int, error) {
	tables, err := tableNames(ctx, d)
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int, len(tables))
	for _, table := range tables {
		var n int
		//nolint:gosec // G202: table is a name read from sqlite_master, quoted as an identifier.
		if err := d.QueryRowContext(ctx, `SELECT count(*) FROM "`+strings.ReplaceAll(table, `"`, `""`)+`"`).Scan(&n); err != nil {
			return nil, fmt.Errorf("counting %s's rows: %w", table, err)
		}
		counts[table] = n
	}
	return counts, nil
}

// tableNames lists d's tables, SQLite's own and a virtual table's shadow
// tables aside.
func tableNames(ctx context.Context, d *sql.DB) ([]string, error) {
	rows, err := d.QueryContext(ctx, `
		SELECT name FROM pragma_table_list
		WHERE schema = 'main' AND type IN ('table', 'virtual') AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("listing tables: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("listing tables: %w", err)
		}
		names = append(names, name)
	}
	return names, rows.Err()
}
