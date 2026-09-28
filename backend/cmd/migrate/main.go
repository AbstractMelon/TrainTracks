// Command migrate applies the SQL migrations in backend/migrations to the
// configured Postgres database (SPEC.md §13, migrate service).
package main

import (
	"errors"
	"log"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"traintracks/internal/config"
)

func main() {
	cfg := config.Default()
	url := cfg.PostgresURL
	if url == "" {
		log.Fatal("POSTGRES_URL is required")
	}

	dir := os.Getenv("MIGRATIONS_DIR")
	if dir == "" {
		dir = "./migrations"
	}

	m, err := migrate.New("file://"+dir, url)
	if err != nil {
		log.Fatalf("init migrate: %v", err)
	}
	defer m.Close()

	command := "up"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	switch command {
	case "up":
		err = m.Up()
	case "down":
		err = m.Down()
	case "version":
		v, dirty, verr := m.Version()
		if verr != nil && !errors.Is(verr, migrate.ErrNilVersion) {
			log.Fatalf("version: %v", verr)
		}
		log.Printf("version=%d dirty=%v", v, dirty)
		return
	default:
		log.Fatalf("unknown command %q (want up|down|version)", command)
	}
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		log.Fatalf("%v: %v", command, err)
	}
	log.Printf("migrate %s: done (no change ok)", command)
}
