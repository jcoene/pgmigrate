// Package pgmigrate performs Postgres database migrations. It aims to be simple, robust, and verbose.
package pgmigrate

import (
	"database/sql"
	"fmt"
	"hash/crc32"
	"log"
	"sort"
	"time"

	_ "github.com/lib/pq"
)

// Migrator contains a database connection and required state to perform migrations.
type Migrator struct {
	url string
	gs  []*Migration
}

// Migration is an individual database migration to be performed.
type Migration struct {
	Version int64
	Name    string
	Up      string
	Down    string
	applied bool
}

// String returns a string that describes the Migration
func (g *Migration) String() string {
	return fmt.Sprintf(`"%d: %s"`, g.Version, g.Name)
}

// NewMigrator creates a new Migrator for the given postgres url.
func NewMigrator(url string) *Migrator {
	return &Migrator{
		url: url,
	}
}

// Add adds Migrations to the Migrator. This method can be called repeatedly
// any time before an Up or Down method is called.
func (m *Migrator) Add(gs ...Migration) *Migrator {
	for i, _ := range gs {
		g := &gs[i]
		m.gs = append(m.gs, g)
	}
	return m
}

// UpOne applies the next pending migration, if any.
func (m *Migrator) UpOne() error {
	return m.apply(1)
}

// UpAll applies all pending migrations, if any.
func (m *Migrator) UpAll() error {
	return m.apply(0)
}

// DownOne reverts the most recently applied migration, if any.
func (m *Migrator) DownOne() error {
	return m.revert(1)
}

// DownAll applies all applied migrations, if any.
func (m *Migrator) DownAll() error {
	return m.revert(0)
}

func (m *Migrator) find(fn func(*Migration) bool) []*Migration {
	gs := []*Migration{}
	for _, g := range m.gs {
		if fn(g) {
			gs = append(gs, g)
		}
	}
	return gs
}

func (m *Migrator) withSession(fn func(db *sql.DB) error) error {
	// establish connection, later close it
	log.Println("migrate: connecting...")
	db, err := sql.Open("postgres", m.url)
	if err != nil {
		return err
	}
	defer func() {
		log.Println("migrate: closing connection...")
		if err := db.Close(); err != nil {
			log.Println("migrate: unable to close connection:", err)
		}
	}()

	// obtain advisory lock, later release it
	lockId := crc32.ChecksumIEEE([]byte(m.url))
	log.Println("migrate: obtaining lock...")
	if _, err := db.Exec(`select pg_advisory_lock($1)`, lockId); err != nil {
		return err
	}
	log.Println("migrate: obtained lock!")
	defer func() {
		log.Println("migrate: releasing lock...")
		if _, err := db.Exec(`select pg_advisory_unlock($1)`, lockId); err != nil {
			log.Println("migrate: unable to release lock:", err)
		}
	}()

	// ensure schema_migrations table exists
	var exists bool
	if err := db.QueryRow(`select exists (select 1 from information_schema.tables where table_name = 'schema_migrations');`).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		log.Println("migrate: schema_migrations table does not exist, creating...")
		if _, err := db.Exec(`create table schema_migrations (version bigint primary key);`); err != nil {
			return err
		}
	}

	// reset migration state
	for _, g := range m.gs {
		g.applied = false
	}

	// update state for applied migrations
	rows, err := db.Query(`select version from schema_migrations order by version asc`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return err
		}
		found := false
		for _, g := range m.gs {
			if v == g.Version {
				g.applied = true
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unable to find migration for schema_migrations version %d! gs: %+v", v, m.gs)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// sort migrations by version
	sort.Slice(m.gs, func(i, j int) bool { return m.gs[i].Version < m.gs[j].Version })

	// execute given function
	return fn(db)
}

func (m *Migrator) apply(n int) error {
	return m.withSession(func(db *sql.DB) error {
		// Get pending up migrations
		gs := m.find(func(g *Migration) bool { return g.applied == false })
		sort.Slice(gs, func(i, j int) bool { return gs[i].Version < gs[j].Version })

		// Apply up to N migrations
		log.Printf("migrate up: there are %d pending migrations.\n", len(gs))
		for i, g := range gs {
			if n > 0 && i >= n {
				break
			}

			t := time.Now()
			log.Printf("migrate up: applying %s...\n", g)

			tx, err := db.Begin()
			if err != nil {
				return err
			}

			if _, err := tx.Exec(g.Up); err != nil {
				log.Printf("migrate up: fatal error error applying %s: %s\n", g, err)
				log.Println("source:", g.Up)
				tx.Rollback()
				return err
			}

			if _, err := tx.Exec(`insert into schema_migrations (version) values ($1)`, g.Version); err != nil {
				log.Printf("migrate up: fatal error error applying %s: %s\n", g, err)
				tx.Rollback()
				return err
			}

			if err := tx.Commit(); err != nil {
				log.Printf("migrate up: fatal error error applying %s: %s\n", g, err)
				tx.Rollback()
				return err
			}

			log.Printf("migrate up: successfully applied %s in %v.\n", g, time.Since(t))
		}

		return nil
	})
}

func (m *Migrator) revert(n int) error {
	return m.withSession(func(db *sql.DB) error {
		// Get pending down migrations
		gs := m.find(func(g *Migration) bool { return g.applied == true })
		sort.Slice(gs, func(i, j int) bool { return gs[i].Version > gs[j].Version })

		// Revert up to N migrations
		log.Printf("migrate down: there are %d applied migrations.\n", len(gs))
		for i, g := range gs {
			if n > 0 && i >= n {
				break
			}

			t := time.Now()
			log.Printf("migrate down: reverting %s...\n", g)

			tx, err := db.Begin()
			if err != nil {
				return err
			}

			if _, err := tx.Exec(g.Down); err != nil {
				log.Printf("migrate down: fatal error error reverting %s: %s\n", g, err)
				log.Println("source:", g.Down)
				tx.Rollback()
				return err
			}

			if _, err := tx.Exec(`delete from schema_migrations where version = $1`, g.Version); err != nil {
				log.Printf("migrate down: fatal error error reverting %s: %s\n", g, err)
				tx.Rollback()
				return err
			}

			if err := tx.Commit(); err != nil {
				log.Printf("migrate down: fatal error error reverting %s: %s\n", g, err)
				tx.Rollback()
				return err
			}

			log.Printf("migrate down: successfully reverted %s in %v.\n", g, time.Since(t))
		}

		return nil
	})
}
