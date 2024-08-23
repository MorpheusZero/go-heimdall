package heimdall

import (
	"os"
	"testing"

	godotenv "github.com/joho/godotenv"
)

func TestHeimdallMigrations(t *testing.T) {

	godotenv.Load("../.env")

	dbConnectionString := os.Getenv("DB_CONNECTION_STRING")

	h := NewHeimdall(dbConnectionString, "migration_history", "./../migrations", true)
	err := h.RunMigrations()
	if err != nil {
		t.Errorf("migrations test failed >> %s", err.Error())
	}
}
