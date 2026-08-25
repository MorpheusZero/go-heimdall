package heimdall

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/joho/godotenv/autoload"
)

func testDBConnectionString(t *testing.T) string {
	t.Helper()
	conn := os.Getenv("DB_CONNECTION_STRING")
	if conn == "" {
		t.Skip("DB_CONNECTION_STRING not set")
	}
	return conn
}

func TestHeimdallMigrations_Success(t *testing.T) {
	ctx := context.Background()
	conn := testDBConnectionString(t)

	config := HeimdallConfig{
		ConnectionString:            conn,
		MigrationTableName:          "migration_history_success",
		MigrationFilesDirectoryPath: "./testdata/migrations_success",
		Verbose:                     false,
	}

	h, err := NewHeimdall(ctx, config)
	if err != nil {
		t.Fatalf("NewHeimdall() error = %v", err)
	}
	defer h.Close(ctx)

	if err := h.RunPGMigrations(ctx); err != nil {
		t.Fatalf("RunPGMigrations() first run error = %v", err)
	}

	if err := h.RunPGMigrations(ctx); err != nil {
		t.Fatalf("RunPGMigrations() second run error = %v", err)
	}
}

func TestHeimdallMigrations_SchemaQualifiedTable(t *testing.T) {
	ctx := context.Background()
	conn := testDBConnectionString(t)

	config := HeimdallConfig{
		ConnectionString:            conn,
		MigrationTableName:          "public.migration_history_schema",
		MigrationFilesDirectoryPath: "./testdata/migrations_schema",
		Verbose:                     false,
	}

	h, err := NewHeimdall(ctx, config)
	if err != nil {
		t.Fatalf("NewHeimdall() error = %v", err)
	}
	defer h.Close(ctx)

	if err := h.RunPGMigrations(ctx); err != nil {
		t.Fatalf("RunPGMigrations() error = %v", err)
	}

	var count int
	err = h.db.QueryRow(ctx, `SELECT COUNT(*) FROM public.migration_history_schema`).Scan(&count)
	if err != nil {
		t.Fatalf("query history table error = %v", err)
	}
	if count == 0 {
		t.Fatal("expected schema-qualified history table to contain rows")
	}
}

func TestHeimdallMigrations_RollbackOnFailure(t *testing.T) {
	ctx := context.Background()
	conn := testDBConnectionString(t)

	config := HeimdallConfig{
		ConnectionString:            conn,
		MigrationTableName:          "migration_history_rollback",
		MigrationFilesDirectoryPath: "./testdata/migrations_rollback",
		Verbose:                     false,
	}

	h, err := NewHeimdall(ctx, config)
	if err != nil {
		t.Fatalf("NewHeimdall() error = %v", err)
	}
	defer h.Close(ctx)

	err = h.RunPGMigrations(ctx)
	if err == nil {
		t.Fatal("RunPGMigrations() expected error, got nil")
	}

	var count int
	err = h.db.QueryRow(ctx, `SELECT COUNT(*) FROM persons_rollback WHERE first_name = 'fourth'`).Scan(&count)
	if err != nil {
		t.Fatalf("query persons error = %v", err)
	}
	if count != 0 {
		t.Fatalf("expected failed migration to roll back, found %d row(s)", count)
	}

	var historyCount int
	err = h.db.QueryRow(ctx, `SELECT COUNT(*) FROM migration_history_rollback`).Scan(&historyCount)
	if err != nil {
		t.Fatalf("query history table error = %v", err)
	}
	if historyCount != 1 {
		t.Fatalf("expected only first migration in history, got %d row(s)", historyCount)
	}
}

func TestValidatePGTableName(t *testing.T) {
	tests := []struct {
		name      string
		tableName string
		wantErr   bool
	}{
		{name: "valid simple name", tableName: "migration_history", wantErr: false},
		{name: "valid schema qualified name", tableName: "public.migration_history", wantErr: false},
		{name: "valid with underscores", tableName: "_my_migrations_2024", wantErr: false},
		{name: "empty name", tableName: "", wantErr: true},
		{name: "invalid characters", tableName: "migration-history", wantErr: true},
		{name: "SQL injection attempt", tableName: "migrations; DROP TABLE users--", wantErr: true},
		{name: "too many qualifiers", tableName: "schema.public.migrations", wantErr: true},
		{name: "starts with number", tableName: "123_migrations", wantErr: true},
		{name: "empty schema part", tableName: ".migrations", wantErr: true},
		{name: "empty table part", tableName: "public.", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePGTableName(tt.tableName)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePGTableName(%q) error = %v, wantErr %v", tt.tableName, err, tt.wantErr)
			}
		})
	}
}

func TestParsePGTableName(t *testing.T) {
	ident, err := parsePGTableName("public.MigrationHistory")
	if err != nil {
		t.Fatalf("parsePGTableName() error = %v", err)
	}
	if got := ident.Sanitize(); got != `"public"."MigrationHistory"` {
		t.Fatalf("Sanitize() = %q, want %q", got, `"public"."MigrationHistory"`)
	}
}

func TestCompareMigrationsToRun(t *testing.T) {
	disk := []string{"001_a.sql", "002_b.sql", "003_c.sql"}
	applied := []string{"001_a.sql", "003_c.sql"}

	got := compareMigrationsToRun(disk, applied)
	if len(got) != 1 || got[0] != "002_b.sql" {
		t.Fatalf("compareMigrationsToRun() = %v, want [002_b.sql]", got)
	}
}

func TestValidateMigrationOrder(t *testing.T) {
	disk := []string{"001_a.sql", "002_b.sql", "003_c.sql"}
	applied := []string{"001_a.sql", "003_c.sql"}

	err := validateMigrationOrder(disk, applied)
	if err == nil {
		t.Fatal("validateMigrationOrder() expected error, got nil")
	}
}

func TestListMigrationFilenames(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"002_b.SQL", "001_a.sql", "notes.txt", "subdir"} {
		path := filepath.Join(dir, name)
		if name == "subdir" {
			if err := os.Mkdir(path, 0o755); err != nil {
				t.Fatalf("Mkdir() error = %v", err)
			}
			continue
		}
		if err := os.WriteFile(path, []byte("-- test"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
	}

	got, err := listMigrationFilenames(dir)
	if err != nil {
		t.Fatalf("listMigrationFilenames() error = %v", err)
	}
	want := []string{"001_a.sql", "002_b.SQL"}
	if len(got) != len(want) {
		t.Fatalf("listMigrationFilenames() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("listMigrationFilenames() = %v, want %v", got, want)
		}
	}
}

func TestReadMigrationFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "001_a.sql"), []byte("SELECT 1;"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	files, err := readMigrationFiles(dir, []string{"001_a.sql"})
	if err != nil {
		t.Fatalf("readMigrationFiles() error = %v", err)
	}
	if len(files) != 1 || files[0].SQL != "SELECT 1;" {
		t.Fatalf("readMigrationFiles() = %+v", files)
	}
}

func TestNewHeimdallValidation(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		config  HeimdallConfig
		wantErr bool
	}{
		{
			name: "empty connection string",
			config: HeimdallConfig{
				ConnectionString:            "",
				MigrationTableName:          "migrations",
				MigrationFilesDirectoryPath: "./sql",
			},
			wantErr: true,
		},
		{
			name: "empty table name",
			config: HeimdallConfig{
				ConnectionString:            "postgres://localhost/db",
				MigrationTableName:          "",
				MigrationFilesDirectoryPath: "./sql",
			},
			wantErr: true,
		},
		{
			name: "empty directory path",
			config: HeimdallConfig{
				ConnectionString:            "postgres://localhost/db",
				MigrationTableName:          "migrations",
				MigrationFilesDirectoryPath: "",
			},
			wantErr: true,
		},
		{
			name: "invalid table name",
			config: HeimdallConfig{
				ConnectionString:            "postgres://localhost/db",
				MigrationTableName:          "bad-name",
				MigrationFilesDirectoryPath: "./sql",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, err := NewHeimdall(ctx, tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewHeimdall() error = %v, wantErr %v", err, tt.wantErr)
			}
			if h != nil {
				h.Close(ctx)
			}
		})
	}
}
