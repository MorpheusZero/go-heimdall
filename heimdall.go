// Package heimdall runs PostgreSQL database migrations from a directory of .sql files.
//
// Migrations run in alphabetical filename order, one transaction per file.
// An advisory lock prevents concurrent runners from applying the same migration twice.
//
// Example:
//
//	ctx := context.Background()
//	config := heimdall.HeimdallConfig{
//		ConnectionString:            "postgres://user:pass@localhost/db",
//		MigrationTableName:          "migration_history",
//		MigrationFilesDirectoryPath: "./migrations",
//		Verbose:                     true,
//	}
//
//	h, err := heimdall.NewHeimdall(ctx, config)
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer h.Close(ctx)
//
//	if err := h.RunPGMigrations(ctx); err != nil {
//		log.Fatal(err)
//	}
package heimdall

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

var pgIdentifierPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// HeimdallConfig holds configuration for a Heimdall instance.
type HeimdallConfig struct {
	// ConnectionString is the PostgreSQL connection string (e.g. "postgres://user:pass@localhost/db").
	ConnectionString string

	// MigrationTableName is the history table. May be schema-qualified (e.g. "public.migration_history").
	MigrationTableName string

	// MigrationFilesDirectoryPath is the directory containing .sql migration files.
	MigrationFilesDirectoryPath string

	// Verbose logs each migration filename and its SQL before execution.
	Verbose bool
}

// Heimdall runs migrations against a single PostgreSQL connection.
// Create with NewHeimdall and call Close when finished.
type Heimdall struct {
	migrationTable          pgx.Identifier
	migrationFilesDirectory string
	db                      *pgx.Conn
	verbose                 bool
}

type migrationFile struct {
	Filename string
	SQL      string
}

// NewHeimdall validates config, connects to PostgreSQL, and returns a Heimdall instance.
func NewHeimdall(ctx context.Context, config HeimdallConfig) (*Heimdall, error) {
	if config.ConnectionString == "" {
		return nil, errors.New("connection string cannot be empty")
	}
	if config.MigrationTableName == "" {
		return nil, errors.New("migration table name cannot be empty")
	}
	if config.MigrationFilesDirectoryPath == "" {
		return nil, errors.New("migration files directory path cannot be empty")
	}

	tableIdent, err := parsePGTableName(config.MigrationTableName)
	if err != nil {
		return nil, fmt.Errorf("invalid migration table name: %w", err)
	}

	conn, err := pgx.Connect(ctx, config.ConnectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return &Heimdall{
		db:                      conn,
		migrationTable:          tableIdent,
		migrationFilesDirectory: config.MigrationFilesDirectoryPath,
		verbose:                 config.Verbose,
	}, nil
}

// Close closes the database connection.
func (h *Heimdall) Close(ctx context.Context) error {
	if h.db != nil {
		return h.db.Close(ctx)
	}
	return nil
}

// RunPGMigrations acquires a migration lock, ensures the history table exists,
// compares disk files to applied migrations, and runs any pending files.
func (h *Heimdall) RunPGMigrations(ctx context.Context) error {
	if err := acquireMigrationLock(ctx, h.db, h.migrationTable); err != nil {
		return fmt.Errorf("failed to acquire migration lock: %w", err)
	}
	defer func() {
		if err := releaseMigrationLock(ctx, h.db, h.migrationTable); err != nil {
			log.Printf("heimdall: failed to release migration lock: %v", err)
		}
	}()

	if err := initializePGMigrationHistoryTable(ctx, h.db, h.migrationTable); err != nil {
		return fmt.Errorf("failed to initialize migration history table: %w", err)
	}

	diskFilenames, err := listMigrationFilenames(h.migrationFilesDirectory)
	if err != nil {
		return fmt.Errorf("failed to list migration files: %w", err)
	}

	appliedFilenames, err := getPGMigrationsInDB(ctx, h.db, h.migrationTable)
	if err != nil {
		return fmt.Errorf("failed to retrieve migration history from database: %w", err)
	}

	if err := validateMigrationOrder(diskFilenames, appliedFilenames); err != nil {
		return err
	}

	pendingFilenames := compareMigrationsToRun(diskFilenames, appliedFilenames)
	if len(pendingFilenames) == 0 {
		if h.verbose {
			log.Println("No new migrations to run")
		}
		return nil
	}

	if h.verbose {
		log.Printf("Running %d migration(s)", len(pendingFilenames))
	}

	migrations, err := readMigrationFiles(h.migrationFilesDirectory, pendingFilenames)
	if err != nil {
		return fmt.Errorf("failed to read migration files: %w", err)
	}

	if err := performPGMigrations(ctx, migrations, h.db, h.migrationTable, h.verbose); err != nil {
		return fmt.Errorf("failed to perform PostgreSQL migrations: %w", err)
	}

	return nil
}

func parsePGTableName(tableName string) (pgx.Identifier, error) {
	if err := validatePGTableName(tableName); err != nil {
		return nil, err
	}
	return pgx.Identifier(strings.Split(tableName, ".")), nil
}

func validatePGTableName(tableName string) error {
	if tableName == "" {
		return errors.New("table name cannot be empty")
	}

	parts := strings.Split(tableName, ".")
	if len(parts) > 2 {
		return errors.New("table name can have at most one schema qualifier (schema.table)")
	}

	for _, part := range parts {
		if part == "" {
			return errors.New("empty identifier in table name")
		}
		if len(part) > 63 {
			return fmt.Errorf("identifier %q exceeds PostgreSQL maximum length of 63 characters", part)
		}
		if !pgIdentifierPattern.MatchString(part) {
			return fmt.Errorf("identifier %q contains invalid characters", part)
		}
	}

	return nil
}

func acquireMigrationLock(ctx context.Context, db *pgx.Conn, table pgx.Identifier) error {
	lockKey := table.Sanitize()
	_, err := db.Exec(ctx, `SELECT pg_advisory_lock(hashtext($1))`, lockKey)
	return err
}

func releaseMigrationLock(ctx context.Context, db *pgx.Conn, table pgx.Identifier) error {
	lockKey := table.Sanitize()
	_, err := db.Exec(ctx, `SELECT pg_advisory_unlock(hashtext($1))`, lockKey)
	return err
}

func initializePGMigrationHistoryTable(ctx context.Context, db *pgx.Conn, table pgx.Identifier) error {
	quotedTable := table.Sanitize()
	createSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id INTEGER GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
			created_at timestamptz NOT NULL DEFAULT now(),
			filename text NOT NULL
		);
	`, quotedTable)
	if _, err := db.Exec(ctx, createSQL); err != nil {
		return err
	}

	// Upgrade existing tables created by older versions of heimdall.
	alterSQL := fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN filename SET NOT NULL`, quotedTable)
	if _, err := db.Exec(ctx, alterSQL); err != nil {
		return err
	}

	indexName := migrationFilenameIndexName(table)
	uniqueSQL := fmt.Sprintf(
		`CREATE UNIQUE INDEX IF NOT EXISTS %s ON %s (filename)`,
		pgx.Identifier{indexName}.Sanitize(),
		quotedTable,
	)
	_, err := db.Exec(ctx, uniqueSQL)
	return err
}

func migrationFilenameIndexName(table pgx.Identifier) string {
	const maxIndexNameLen = 63
	base := "heimdall_" + strings.Join(table, "_") + "_filename_key"
	if len(base) <= maxIndexNameLen {
		return base
	}
	return base[:maxIndexNameLen]
}

func listMigrationFilenames(migrationsDir string) ([]string, error) {
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %q: %w", migrationsDir, err)
	}

	var filenames []string
	for _, entry := range entries {
		if entry.IsDir() || !isSQLFile(entry.Name()) {
			continue
		}
		filenames = append(filenames, entry.Name())
	}

	sort.Strings(filenames)
	return filenames, nil
}

func isSQLFile(name string) bool {
	return strings.EqualFold(filepath.Ext(name), ".sql")
}

func readMigrationFiles(migrationsDir string, filenames []string) ([]migrationFile, error) {
	migrations := make([]migrationFile, 0, len(filenames))
	for _, filename := range filenames {
		content, err := os.ReadFile(filepath.Join(migrationsDir, filename))
		if err != nil {
			return nil, fmt.Errorf("failed to read migration file %q: %w", filename, err)
		}
		migrations = append(migrations, migrationFile{
			Filename: filename,
			SQL:      string(content),
		})
	}
	return migrations, nil
}

func getPGMigrationsInDB(ctx context.Context, db *pgx.Conn, table pgx.Identifier) ([]string, error) {
	sql := fmt.Sprintf(`SELECT filename FROM %s ORDER BY created_at ASC`, table.Sanitize())
	rows, err := db.Query(ctx, sql)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var filenames []string
	for rows.Next() {
		var filename string
		if err := rows.Scan(&filename); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		filenames = append(filenames, filename)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return filenames, nil
}

// compareMigrationsToRun returns disk filenames that are not yet recorded in the history table.
func compareMigrationsToRun(diskFilenames, appliedFilenames []string) []string {
	applied := make(map[string]struct{}, len(appliedFilenames))
	for _, filename := range appliedFilenames {
		applied[filename] = struct{}{}
	}

	var pending []string
	for _, filename := range diskFilenames {
		if _, ok := applied[filename]; !ok {
			pending = append(pending, filename)
		}
	}
	return pending
}

// validateMigrationOrder rejects gaps where a later migration was applied before an earlier one.
func validateMigrationOrder(diskFilenames, appliedFilenames []string) error {
	applied := make(map[string]struct{}, len(appliedFilenames))
	for _, filename := range appliedFilenames {
		applied[filename] = struct{}{}
	}

	for i, filename := range diskFilenames {
		if _, ok := applied[filename]; ok {
			continue
		}
		for _, later := range diskFilenames[i+1:] {
			if _, ok := applied[later]; ok {
				return fmt.Errorf(
					"out-of-order migration detected: %q is missing but later migration %q is already applied",
					filename,
					later,
				)
			}
		}
	}

	return nil
}

func performPGMigrations(ctx context.Context, migrations []migrationFile, db *pgx.Conn, table pgx.Identifier, verbose bool) error {
	quotedTable := table.Sanitize()
	insertSQL := fmt.Sprintf(`INSERT INTO %s (filename) VALUES ($1)`, quotedTable)

	for _, migration := range migrations {
		if verbose {
			log.Printf("Executing migration: %s", migration.Filename)
			log.Println(migration.SQL)
			log.Println("---")
		}

		tx, err := db.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return fmt.Errorf("failed to begin transaction for migration %q: %w", migration.Filename, err)
		}

		_, err = tx.Exec(ctx, migration.SQL)
		if err != nil {
			if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
				return fmt.Errorf(
					"failed to execute migration %q: %w (rollback failed: %v)",
					migration.Filename,
					err,
					rbErr,
				)
			}
			return fmt.Errorf("failed to execute migration %q: %w", migration.Filename, err)
		}

		_, err = tx.Exec(ctx, insertSQL, migration.Filename)
		if err != nil {
			if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
				return fmt.Errorf(
					"failed to record migration %q in history table: %w (rollback failed: %v)",
					migration.Filename,
					err,
					rbErr,
				)
			}
			return fmt.Errorf("failed to record migration %q in history table: %w", migration.Filename, err)
		}

		if err = tx.Commit(ctx); err != nil {
			return fmt.Errorf("failed to commit transaction for migration %q: %w", migration.Filename, err)
		}

		if verbose {
			log.Printf("Successfully applied migration: %s", migration.Filename)
		}
	}

	return nil
}
