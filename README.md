# pgmigrate

[![Build Status](https://secure.travis-ci.org/jcoene/pgmigrate.png?branch=master)](http://travis-ci.org/jcoene/pgmigrate) [![GoDoc](https://godoc.org/github.com/jcoene/pgmigrate?status.svg)](http://godoc.org/github.com/jcoene/pgmigrate)

Pgmigrate is a library that performs database migrations. It only supports Postgres and aims to be simple, robust, and verbose.

## Why?

There are plenty of other database migration solutions for Go. Over the years they have seen a dramatic increase in the number of databases supported, features, etc. This results in tradeoffs and complexity. I wanted something that was easy to use and debug, built for the known tradeoffs associated with Postgres, so here we are.

## Choices

- It supports only Postgres.
- It's modeled loosely after ActiveRecord migrations. Migrations are versioned and are applied or reverted in order.
- Migrations are performed inside of transactions. A migration will either succeed or fail, but will not partially suceed or leave the database in a dirty state.
- Migrations are defined in code, not files on disk. This is done to ease testing and deployment.


## Usage

Apply migrations:

```go

import "github.com/jcoene/pgmigrate"

// Create a new migrator with a Postgres URL
m := pgmigrate.NewMigrator("postgres://postgres:@127.0.0.1:5432/myapp_development?sslmode=disable")

// Add some migration definitions
m.Add(&pgmigrate.Migration{
  Version: 1,
  Name:    "widgets_init",
  Up: `
    create table widgets (
      widget_id integer primary key,
      name text
    );
  `,
  Down: `
    drop table if exists widgets;
  `,
})
m.Add(&pgmigrate.Migration{
  Version: 2,
  Name:    "users_init",
  Up: `
    create table users (
      user_id integer primary key,
      name text
    );

    alter table widgets add column user_id integer references widgets;
  `,
  Down: `
    alter table widgets drop column user_id;
    drop table if exists users;
  `,
})

// Run all migrations to reach the desired state. This only applies pending
// migrations and is idempotent (so long as your migrations are sensible).
if err := m.UpAll(); err != nil {
  log.Fatal(err)
}
```

See the test file or godoc for more details.

## License

MIT License, see [LICENSE](https://github.com/jcoene/pgmigrate/blob/master/LICENSE)
