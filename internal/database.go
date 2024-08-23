package database

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
)

type HeimdallDatabase struct {
	Connection *pgx.Conn
}

func NewHeimdallDatabase(connectionString string) *HeimdallDatabase {
	conn, err := pgx.Connect(context.Background(), connectionString)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to connect to database: %v\n", err)
		panic(err)
	}
	return &HeimdallDatabase{Connection: conn}
}
