# go-heimdall

A small database migration tool written in Golang for Postgres that follows [KISS](https://en.wikipedia.org/wiki/KISS_principle).

![Static Badge](https://img.shields.io/badge/pkg.go.dev-reference-blue?style=flat-square&logo=go&link=https%3A%2F%2Fpkg.go.dev%2Fgithub.com%2Fmorpheuszero%2Fgo-heimdall)

## Features

- Heimdall will handle basic DB migrations for Postgres using minimal dependencies.
- Heimdall will perform each migration in a transaction--if the transaction fails, the SQL will be rolled back and the app will panic.
- You have a few configuration options available to you for naming your migrations table as you see fit and also the directory where you store your migrations files.
- This tool is **NOT** a binary and is meant to be used as a dependency in your own project. You can create your own binary though if you so choose by forking this project.
  - You should create a command in your own application that wil invoke the code as shown in the examples below.

## Install

```shell
go get -u github.com/morpheuszero/go-heimdall/v4
```

## Usage

Pre-reqs:

- You must have a flat folder on disk somewhere with all of your .sql migration files in them. They will be loaded in alphabetical order by filename, so it's recommended to name by date like `20240722_v1_a_short_description.sql`

Steps:

- Create a Config with the following parameters:
  - Database Connection String
  - Migration History Table Name (can be schema-qualified, e.g., "public.migration_history")
  - Migrations Directory that holds all of your .SQL files
  - VERBOSE = TRUE/FALSE (if true it will show the SQL being ran when the migrations run)
- Create a new Heimdall instance with the config
- Run the migrations
- Close the connection when done
- Smile =)

```go
import (
	heimdall "github.com/morpheuszero/go-heimdall/v4"
)

config := heimdall.HeimdallConfig{
	ConnectionString:            dbConnectionString,
	MigrationTableName:          "migration_history",
	MigrationFilesDirectoryPath: "./migrations",
	Verbose:                     true,
}

h, err := heimdall.NewHeimdall(config)
if err != nil {
	log.Fatal(err)
}
defer h.Close()

err = h.RunPGMigrations()
if err != nil {
	log.Fatal(err)
}
```

## Developing Locally

There is an included `.env.example` file here for loading your database connection for testing locally.

In a real application, we don't load the ENV vars for you, and its expected for the users of the package to supply the connection string.
