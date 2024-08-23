# go-heimdall

A small database migration tool in Golang for Postgres that follows KISS (Keep It Simple Stupid).

Heimdall will handle very basic DB migrations for Postgres using minimal dependencies and very basic use cases.

## Install

```shell
go get -u github.com/morpheuszero/go-heimdall
```

## Usage

Pre-reqs:

- You must have a flat folder on disk somewhere with all of your .sql migration files in them. They will be loaded in order based on their filenames, as such, its recommended to name by date like `20240722_v1_a_short_description.sql`

Steps:

- Create a new Heimdall Instance with following parameters:
  - Database Connection String
  - Migration History Table Name
  - Migrations Directory that holds all of your .SQL files
  - VERBOSE = TRUE/FALSE (if true it will show the SQL being ran when the migrations run)
- Run the migrations
- Smile =)

```go
	import (
        heimdall "github.com/morpheuszero/go-heimdall/pkg"
    )

	h := heimdall.NewHeimdall(dbConnectionString, "migration_history", "./migrations", true)
	err := h.RunMigrations()
```
