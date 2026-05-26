package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/hashmap-kz/pgsize/internal/ui"

	"github.com/hashmap-kz/pgsize/internal/pg"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	dsn := flag.String("dsn",
		"postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable",
		"Postgres connection string (or use PG* env vars)",
	)
	flag.Parse()

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "ping: %v\n", err)
		os.Exit(1)
	}

	dbs, err := pg.ListDatabases(ctx, pool)
	if err != nil {
		fmt.Fprintf(os.Stderr, "list databases: %v\n", err)
		os.Exit(1)
	}

	app := ui.InitialModel(pool, dbs, *dsn)
	p := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "tui: %v\n", err)
		os.Exit(1)
	}
}
