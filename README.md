# go-heimdall

<p align="center">
  <img src="heimdall.jpg" alt="Heimdall" width="180" />
</p>

A small PostgreSQL migration library for Go that follows [KISS](https://en.wikipedia.org/wiki/KISS_principle).

[![pkg.go.dev reference](https://img.shields.io/badge/pkg.go.dev-reference-blue?style=flat-square&logo=go)](https://pkg.go.dev/github.com/morpheuszero/go-heimdall/v4)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue?style=flat-square)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-1.27+-00ADD8?style=flat-square&logo=go)](https://go.dev/dl/)

## Public Disclosure

I originally wrote the first few versions of this library by hand **without** the use of AI. After I got it to a spot that I liked, for v4, I did use AI to help me identify bugs and offer suggestions on how to improve resilience and performance. This was all a labor of love and I used AI as a tool in helping me to generate some documentation and offer some aid with bug fixes. This tool has always been a personal tool that I use on my own projects for simplicity. If you find it useful in any capacity, feel free to use it, fork it, contribute, etc.

## Features

- Runs `.sql` migration files from a flat directory in alphabetical order.
- Wraps each migration in a transaction; failures roll back that file's changes.
- Uses a PostgreSQL advisory lock so concurrent runners do not double-apply migrations.
- Supports schema-qualified history table names (e.g. `public.migration_history`).
- Intended as a library dependency, not a standalone CLI.

## Install

```shell
go get -u github.com/morpheuszero/go-heimdall/v4
```

## Usage

1. Place migration files in a single directory. Name them so alphabetical order matches apply order (for example `20240722_add_users.sql`).
2. Configure and run migrations:

```go
import (
	"context"
	"log"

	heimdall "github.com/morpheuszero/go-heimdall/v4"
)

ctx := context.Background()

config := heimdall.HeimdallConfig{
	ConnectionString:            dbConnectionString,
	MigrationTableName:          "migration_history",
	MigrationFilesDirectoryPath: "./migrations",
	Verbose:                     true,
}

h, err := heimdall.NewHeimdall(ctx, config)
if err != nil {
	log.Fatal(err)
}
defer h.Close(ctx)

if err := h.RunPGMigrations(ctx); err != nil {
	log.Fatal(err)
}
```

### Configuration

| Field | Description |
| --- | --- |
| `ConnectionString` | PostgreSQL connection string |
| `MigrationTableName` | History table name; may be schema-qualified |
| `MigrationFilesDirectoryPath` | Directory containing `.sql` files |
| `Verbose` | Log each migration filename and SQL body |

### Migration file rules

- Only files ending in `.sql` (case-insensitive) are executed.
- Each file runs inside a single transaction.
- Avoid `COMMIT`, `CREATE INDEX CONCURRENTLY`, and `VACUUM` inside migration files; they conflict with the per-file transaction wrapper.
- If a later migration is already applied while an earlier one is missing, heimdall returns an out-of-order error instead of applying the gap.

## Developing locally

Copy `.env.example` to `.env` and set `DB_CONNECTION_STRING`, then start Postgres:

```shell
docker compose up -d
go test ./...
```

Integration tests are skipped when `DB_CONNECTION_STRING` is not set.
