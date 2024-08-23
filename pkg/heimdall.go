package heimdall

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	database "github.com/morpheuszero/go-heimdall/internal"
)

type Heimdall struct {
	migrationTableName          string
	migrationFilesDirectoryPath string
	db                          *database.HeimdallDatabase
	verbose                     bool
}

type MigrationFile struct {
	Filename string
	SQL      string
}

// Creates a new Heimdall instance.
//
// Handles creating the database connection and setting some values that we will use.
func NewHeimdall(connectionString string, migrationTableName string, migrationFilesDirectoryPath string, verbose bool) *Heimdall {
	db := database.NewHeimdallDatabase(connectionString)
	return &Heimdall{db: db, migrationTableName: migrationTableName, migrationFilesDirectoryPath: migrationFilesDirectoryPath, verbose: verbose}
}

// Run the migrations
func (h *Heimdall) RunMigrations() error {
	err := initializeMigrationHistoryTable(h.db, h.migrationTableName)
	if err != nil {
		log.Fatalln("an error occurred when attempting to create the migrations history table")
		return err
	}
	migrationFiles, err := getAllMigrationFiles(h.migrationFilesDirectoryPath)
	if err != nil {
		log.Fatalln("an error occurred when attempting to retrieve all migration files from the disk")
		return err
	}
	migrationsInDB, err := getMigrationsInDB(h.db, h.migrationTableName)
	if err != nil {
		log.Fatalln("an error occurred when attempting to retrieve all migration history from the db")
		return err
	}
	migrationsToRun := compareMigrationsToRun(migrationFiles, migrationsInDB)
	err = performMigrations(migrationsToRun, h.db, h.migrationTableName, h.verbose)
	if err != nil {
		log.Fatalln("an error occurred when attempting to perform the migrations")
		return err
	}
	return nil
}

func initializeMigrationHistoryTable(db *database.HeimdallDatabase, migrationTableName string) error {
	sql := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s" (
		"id" INTEGER GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
		"created_at" timestamp DEFAULT (now()),
		"filename" text
		);
	`, migrationTableName)
	_, err := db.Connection.Exec(context.Background(), sql)
	if err != nil {
		return err
	}
	return nil
}

func getAllMigrationFiles(migrationFilesDirectoryPath string) ([]MigrationFile, error) {
	files, err := os.ReadDir(migrationFilesDirectoryPath)
	if err != nil {
		return nil, err
	}

	var migrationFiles []MigrationFile

	for _, file := range files {
		if filepath.Ext(file.Name()) == ".sql" {
			filePath := filepath.Join(migrationFilesDirectoryPath, file.Name())

			content, err := os.ReadFile(filePath)
			if err != nil {
				log.Printf("failed to read file %s: %v", filePath, err)
				continue
			}

			migrationFiles = append(migrationFiles, *(&MigrationFile{
				Filename: file.Name(),
				SQL:      string(content),
			}))
		}
	}
	return migrationFiles, nil
}

func getMigrationsInDB(db *database.HeimdallDatabase, migrationTableName string) ([]string, error) {
	sql := fmt.Sprintf(`
		SELECT filename 
			FROM public.%s
		ORDER BY created_at ASC 
	`, migrationTableName)
	rows, err := db.Connection.Query(context.Background(), sql)
	if err != nil {
		return nil, err
	}

	var filenames []string

	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			log.Fatalf("Failed to scan row: %v", err)
		}
		filenames = append(filenames, value)
	}

	return filenames, nil
}

func compareMigrationsToRun(migrationFiles []MigrationFile, migrationsInDB []string) []MigrationFile {
	var migrationsToRun []MigrationFile

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

func performMigrations(migrations []MigrationFile, db *database.HeimdallDatabase, migrationTableName string, verbose bool) error {
	for _, migration := range migrations {

		if verbose {
			fmt.Println(migration.SQL)
		}

		_, err := db.Connection.Exec(context.Background(), migration.SQL)
		if err != nil {
			return err
		} else {
			sql := fmt.Sprintf(`
				INSERT INTO public.%s (
					filename
				) 
				VALUES ($1)
		`, migrationTableName)
			_, err = db.Connection.Exec(context.Background(), sql, migration.Filename)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
