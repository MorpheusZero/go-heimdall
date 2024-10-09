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
go get -u github.com/morpheuszero/go-heimdall@v1.1.0
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
        heimdall "github.com/morpheuszero/go-heimdall"
    )

	h := heimdall.NewHeimdall(dbConnectionString, "migration_history", "./migrations", true)
	err := h.RunMigrations()
```

## Developing Locally

There is an included `.env.example` file here for loading your database connection for testing locally.
In a real application, we don't load the ENV vars for you, and its expected for the users of the package
to supply the connection string.
