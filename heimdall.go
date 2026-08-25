// Package heimdall implements utility functions for managing database
// migrations for an application.
//
// The heimdall package works with PostgresSQL databases.
//
// Example usage:
//
//	config := heimdall.HeimdallConfig{
//		ConnectionString:            "postgres://user:pass@localhost/db",
//		MigrationTableName:          "migration_history",
//		MigrationFilesDirectoryPath: "./migrations",
//		Verbose:                     true,
//	}
//
//	h, err := heimdall.NewHeimdall(config)
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer h.Close()
//
//	// Run PostgreSQL migrations
//	if err := h.RunPGMigrations(); err != nil {
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

// HeimdallConfig holds the configuration parameters for creating a new Heimdall instance.
type HeimdallConfig struct {
	// ConnectionString is the PostgreSQL connection string (e.g., "postgres://user:pass@localhost/db")
	ConnectionString string

	// MigrationTableName is the name of the table that will store migration history.
	// Can be schema-qualified (e.g., "public.migrations" or "myschema.migration_history").
	// If not schema-qualified, uses the default schema.
	MigrationTableName string

	// MigrationFilesDirectoryPath is the path to the directory containing .sql migration files.
	// Files will be executed in alphabetical order by filename.
	MigrationFilesDirectoryPath string

	// Verbose enables detailed logging of migration SQL being executed.
	Verbose bool
}

// Heimdall is the main instance that handles running database migrations.
// Use NewHeimdall to create an instance, and always call Close() when done.
type Heimdall struct {
	migrationTableName          string    // The name of the table that Heimdall will create in your database to store information about the migration history
	migrationFilesDirectoryPath string    // The relative path of the directory that holds all of your .sql files that should be run.
	db                          *pgx.Conn // A reference to the active PostgreSQL database connection
	verbose                     bool      // If TRUE, will output more logging information about the migrations being ran
}

// migrationFile represents a SQL migration file in your specified migrations directory.
type migrationFile struct {
	Filename string // The name of the file on disk
	SQL      string // The actual SQL content that will be run on the server
}

// NewHeimdall creates a new Heimdall instance with the provided configuration.
// Returns an error if the configuration is invalid or database connection fails.
// Always call Close() on the returned Heimdall instance when done to clean up resources.
func NewHeimdall(config HeimdallConfig) (*Heimdall, error) {
	// Validate configuration
	if config.ConnectionString == "" {
		return nil, errors.New("connection string cannot be empty")
	}
	if config.MigrationTableName == "" {
		return nil, errors.New("migration table name cannot be empty")
	}
	if config.MigrationFilesDirectoryPath == "" {
		return nil, errors.New("migration files directory path cannot be empty")
	}

	// Validate table name format
	if err := validatePGTableName(config.MigrationTableName); err != nil {
		return nil, fmt.Errorf("invalid migration table name: %w", err)
	}

	// Create database connection
	conn, err := pgx.Connect(context.Background(), config.ConnectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return &Heimdall{
		db:                          conn,
		migrationTableName:          config.MigrationTableName,
		migrationFilesDirectoryPath: config.MigrationFilesDirectoryPath,
		verbose:                     config.Verbose,
	}, nil
}

// Close closes the database connection. Should be called when done using Heimdall.
func (h *Heimdall) Close() error {
	if h.db != nil {
		return h.db.Close(context.Background())
	}
	return nil
}

// validatePGTableName validates that a table name follows PostgreSQL identifier rules.
// Supports schema-qualified names (e.g., "schema.table").
func validatePGTableName(tableName string) error {
	if tableName == "" {
		return errors.New("table name cannot be empty")
	}

	// Split by dot to handle schema-qualified names
	parts := strings.Split(tableName, ".")
	if len(parts) > 2 {
		return errors.New("table name can have at most one schema qualifier (schema.table)")
	}

	// PostgreSQL identifier pattern: starts with letter or underscore, followed by alphanumeric or underscore
	identifierPattern := regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

	for _, part := range parts {
		if len(part) == 0 {
			return errors.New("empty identifier in table name")
		}
		if len(part) > 63 {
			return fmt.Errorf("identifier '%s' exceeds PostgreSQL maximum length of 63 characters", part)
		}
		if !identifierPattern.MatchString(part) {
			return fmt.Errorf("identifier '%s' contains invalid characters (must start with letter/underscore, contain only alphanumeric and underscores)", part)
		}
	}

	return nil
}

// RunMPGigrations runs the entire migration process for a PostgreSQL database.
// It initializes the migration history table, reads migration files from disk,
// determines which migrations need to be run, and executes them in transactions.
// Returns an error if any step fails.
func (h *Heimdall) RunPGMigrations() error {

	if err := initializePGMigrationHistoryTable(h.db, h.migrationTableName); err != nil {
		return fmt.Errorf("failed to initialize migration history table: %w", err)
	}

	migrationFiles, err := getAllMigrationFiles(h.migrationFilesDirectoryPath)
	if err != nil {
		return fmt.Errorf("failed to read migration files: %w", err)
	}

	migrationsInDB, err := getPGMigrationsInDB(h.db, h.migrationTableName)
	if err != nil {
		return fmt.Errorf("failed to retrieve migration history from database: %w", err)
	}

	migrationsToRun := compareMigrationsToRun(migrationFiles, migrationsInDB)

	if len(migrationsToRun) == 0 {
		if h.verbose {
			log.Println("No new migrations to run")
		}
		return nil
	}

	if h.verbose {
		log.Printf("Running %d migration(s)\n", len(migrationsToRun))
	}

	if err := performPGMigrations(migrationsToRun, h.db, h.migrationTableName, h.verbose); err != nil {
		return fmt.Errorf("failed to perform PostgreSQL migrations: %w", err)
	}

	return nil
}

// initializePGMigrationHistoryTable will attempt to create the migrations history table if it does not exist.
func initializePGMigrationHistoryTable(db *pgx.Conn, migrationTableName string) error {
	sql := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s" (
		"id" INTEGER GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
		"created_at" timestamp DEFAULT (now()),
		"filename" text
		);
	`, migrationTableName)
	_, err := db.Exec(context.Background(), sql)
	if err != nil {
		return err
	}
	return nil
}

// getAllMigrationFiles reads the given directory path to load all .sql files and read their contents.
// Files are sorted alphabetically by filename to ensure deterministic execution order.
// Returns an error if the directory cannot be read, any file cannot be read, or duplicate filenames are found.
func getAllMigrationFiles(migrationFilesDirectoryPath string) ([]migrationFile, error) {
	files, err := os.ReadDir(migrationFilesDirectoryPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory '%s': %w", migrationFilesDirectoryPath, err)
	}

	var migrationFiles []migrationFile
	seenFiles := make(map[string]bool)

	for _, file := range files {
		// Skip non-SQL files and directories
		if file.IsDir() || filepath.Ext(file.Name()) != ".sql" {
			continue
		}

		// Check for duplicate filenames
		if seenFiles[file.Name()] {
			return nil, fmt.Errorf("duplicate migration file found: %s", file.Name())
		}
		seenFiles[file.Name()] = true

		filePath := filepath.Join(migrationFilesDirectoryPath, file.Name())

		// Read file content - fail on any error
		content, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read migration file '%s': %w", filePath, err)
		}

		migrationFiles = append(migrationFiles, migrationFile{
			Filename: file.Name(),
			SQL:      string(content),
		})
	}

	// Sort files alphabetically by filename to ensure deterministic execution order
	sort.Slice(migrationFiles, func(i, j int) bool {
		return migrationFiles[i].Filename < migrationFiles[j].Filename
	})

	return migrationFiles, nil
}

// getPGMigrationsInDB retrieves the list of migration files that have already been executed.
// Returns filenames in the order they were applied (sorted by created_at).
func getPGMigrationsInDB(db *pgx.Conn, migrationTableName string) ([]string, error) {
	sql := fmt.Sprintf(`
		SELECT filename 
			FROM %s
		ORDER BY created_at ASC 
	`, migrationTableName)
	rows, err := db.Query(context.Background(), sql)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var filenames []string

	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		filenames = append(filenames, value)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return filenames, nil
}

// compareMigrationsToRun will compare the list of files to the migrations found in the db to see which files need to be run.
// Returns only the filtered diff slice.
func compareMigrationsToRun(migrationFiles []migrationFile, migrationsInDB []string) []migrationFile {
	var migrationsToRun []migrationFile

	for _, migration := range migrationFiles {
		found := false
		for _, migrationsInDBName := range migrationsInDB {
			if migration.Filename == migrationsInDBName {
				found = true
			}
		}
		if !found {
			migrationsToRun = append(migrationsToRun, migration)
		}
	}
	return migrationsToRun
}

// performPGMigrations executes the provided migrations in individual transactions.
// Each migration is run atomically: if it fails, the transaction is rolled back.
// If any migration fails, the entire process stops and returns an error.
func performPGMigrations(migrations []migrationFile, db *pgx.Conn, migrationTableName string, verbose bool) error {
	for _, migration := range migrations {
		if verbose {
			log.Printf("Executing migration: %s\n", migration.Filename)
			fmt.Println(migration.SQL)
			fmt.Println("---")
		}

		// Begin a transaction for this migration
		tx, err := db.BeginTx(context.Background(), pgx.TxOptions{})
		if err != nil {
			return fmt.Errorf("failed to begin transaction for migration '%s': %w", migration.Filename, err)
		}

		// Execute the migration SQL
		_, err = tx.Exec(context.Background(), migration.SQL)
		if err != nil {
			tx.Rollback(context.Background())
			return fmt.Errorf("failed to execute migration '%s': %w", migration.Filename, err)
		}

		// Record the migration in the history table
		sql := fmt.Sprintf(`
			INSERT INTO %s (
				filename
			) 
			VALUES ($1)
		`, migrationTableName)
		_, err = tx.Exec(context.Background(), sql, migration.Filename)
		if err != nil {
			tx.Rollback(context.Background())
			return fmt.Errorf("failed to record migration '%s' in history table: %w", migration.Filename, err)
		}

		// Commit the transaction
		if err = tx.Commit(context.Background()); err != nil {
			return fmt.Errorf("failed to commit transaction for migration '%s': %w", migration.Filename, err)
		}

		if verbose {
			log.Printf("Successfully applied migration: %s\n", migration.Filename)
		}
	}
	return nil
}
