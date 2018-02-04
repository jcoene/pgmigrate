package pgmigrate

import (
	"math/rand"
	"os"
	"testing"
)

func TestPgMigrate(t *testing.T) {
	postgresUrl := os.Getenv("POSTGRES_URL")
	if postgresUrl == "" {
		postgresUrl = "postgres://postgres:@127.0.0.1:5432/pgmigrate_test?sslmode=disable"
	}

	m := NewMigrator(postgresUrl)
	m.Add(Migration{
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
	m.Add(Migration{
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
	m.Add(Migration{
		Version: 3,
		Name:    "users_add_birthday",
		Up: `
			alter table users add column birthday date;
			create index users_birthday on users (birthday);
		`,
		Down: `
			alter table users drop column birthday;
		`,
	})

	if err := m.DownAll(); err != nil {
		t.Fatalf("unexpected error running DownAll: %s", err)
	}

	var funcs = []struct {
		name string
		fn   func() error
	}{
		{name: "DownAll", fn: m.DownAll},
		{name: "UpAll", fn: m.UpAll},
		{name: "UpOne", fn: m.UpOne},
		{name: "UpOne", fn: m.UpOne},
		{name: "UpOne", fn: m.UpOne},
		{name: "UpOne", fn: m.UpOne},
		{name: "UpOne", fn: m.UpOne},
		{name: "DownOne", fn: m.DownOne},
		{name: "DownOne", fn: m.DownOne},
		{name: "DownOne", fn: m.DownOne},
		{name: "DownOne", fn: m.DownOne},
		{name: "DownOne", fn: m.DownOne},
	}

	for i := 0; i < 100; i++ {
		n := rand.Intn(len(funcs))
		if err := funcs[n].fn(); err != nil {
			t.Fatalf("unexpected error running %s (random): %s", funcs[n].name, err)
		}
	}

	if err := m.UpAll(); err != nil {
		t.Fatalf("unexpected error running UpAll: %s", err)
	}
}
