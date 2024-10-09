package heimdall

import (
	"os"
	"testing"

	_ "github.com/joho/godotenv/autoload"
)

func TestHeimdallMigrations(t *testing.T) {

	dbConnectionString := os.Getenv("DB_CONNECTION_STRING")

	h := NewHeimdall(dbConnectionString, "migration_history", "./migrations_examples", true)
	err := h.RunMigrations()
	if err != nil {
		t.Errorf("migrations test failed >> %s", err.Error())
	}
}
