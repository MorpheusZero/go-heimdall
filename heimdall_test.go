package heimdall

import (
	"os"
	"testing"

	_ "github.com/joho/godotenv/autoload"
)

func TestHeimdallMigrations(t *testing.T) {
	dbConnectionString := os.Getenv("DB_CONNECTION_STRING")

	config := HeimdallConfig{
		ConnectionString:            dbConnectionString,
		MigrationTableName:          "migration_history",
		MigrationFilesDirectoryPath: "./migrations_examples",
		Verbose:                     true,
	}

	h, err := NewHeimdall(config)
	if err != nil {
		t.Fatalf("failed to create Heimdall instance: %v", err)
	}
	defer h.Close()

	err = h.RunPGMigrations()
	if err != nil {
		t.Errorf("PostgreSQL migrations test failed: %v", err)
	}
}

func TestValidatePGTableName(t *testing.T) {
	tests := []struct {
		name      string
		tableName string
		wantErr   bool
	}{
		{
			name:      "valid simple name",
			tableName: "migration_history",
			wantErr:   false,
		},
		{
			name:      "valid schema qualified name",
			tableName: "public.migration_history",
			wantErr:   false,
		},
		{
			name:      "valid with underscores",
			tableName: "_my_migrations_2024",
			wantErr:   false,
		},
		{
			name:      "empty name",
			tableName: "",
			wantErr:   true,
		},
		{
			name:      "invalid characters",
			tableName: "migration-history",
			wantErr:   true,
		},
		{
			name:      "SQL injection attempt",
			tableName: "migrations; DROP TABLE users--",
			wantErr:   true,
		},
		{
			name:      "too many qualifiers",
			tableName: "schema.public.migrations",
			wantErr:   true,
		},
		{
			name:      "starts with number",
			tableName: "123_migrations",
			wantErr:   true,
		},
		{
			name:      "empty schema part",
			tableName: ".migrations",
			wantErr:   true,
		},
		{
			name:      "empty table part",
			tableName: "public.",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePGTableName(tt.tableName)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateTableName(%q) error = %v, wantErr %v", tt.tableName, err, tt.wantErr)
			}
		})
	}
}

func TestNewHeimdallValidation(t *testing.T) {
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
			name: "invalid table name is allowed until migrations run",
			config: HeimdallConfig{
				ConnectionString:            "postgres://localhost/db",
				MigrationTableName:          "bad-name",
				MigrationFilesDirectoryPath: "./sql",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, err := NewHeimdall(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewHeimdall() error = %v, wantErr %v", err, tt.wantErr)
			}
			if h != nil {
				h.Close()
			}
		})
	}
}
