// Command migrate applies or rolls back shop_ingestor's schema migrations.
// Never run this against production from a local machine — see
// shop_docs/docs/local-development.md.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/hegarty/shop_platform/config"
	"github.com/hegarty/shop_platform/db"

	"github.com/hegarty/shop_ingestor/migrations"
)

func main() {
	direction := flag.String("direction", "up", "up | down")
	steps := flag.Int("steps", 1, "number of migrations to roll back (down only)")
	flag.Parse()

	l := config.NewLoader()
	dbHost := l.String("DATABASE_HOST")
	dbPort := l.IntDefault("DATABASE_PORT", 5432)
	dbName := l.String("DATABASE_NAME")
	dbUser := l.String("DATABASE_USER")
	dbPassword := l.String("DATABASE_PASSWORD")
	if err := l.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := db.Connect(ctx, db.Config{Host: dbHost, Port: dbPort, Database: dbName, User: dbUser}, dbPassword)
	if err != nil {
		fmt.Fprintln(os.Stderr, "connect:", err)
		os.Exit(1)
	}
	defer pool.Close()

	loaded, err := db.LoadMigrations(migrations.FS)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load migrations:", err)
		os.Exit(1)
	}

	switch *direction {
	case "up":
		err = db.Migrate(ctx, pool, loaded)
	case "down":
		err = db.Rollback(ctx, pool, loaded, *steps)
	default:
		fmt.Fprintf(os.Stderr, "unknown direction %q\n", *direction)
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
	fmt.Println("migrations applied successfully")
}
