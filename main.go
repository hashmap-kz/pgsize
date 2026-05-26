package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/hashmap-kz/pgsize/internal/ui"
	"github.com/hashmap-kz/pgsize/internal/x/fmtx"

	"github.com/hashmap-kz/pgsize/internal/pg"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jackc/pgx/v5/pgxpool"
)

var Version = "dev"

//nolint:gosec // false positive: example URL in usage text, not real credentials
var usage = `pgsize - interactive TUI for exploring PostgreSQL database sizes

Browse databases, schemas, tables, and index sizes in a terminal UI.
Connect with a DSN or rely on libpq environment variables (PGHOST, PGUSER, etc.).

Usage:
  pgsize [flags]

Examples:
  pgsize
  pgsize --dsn "postgres://user:pass@localhost/mydb"
  pgsize --dsn "$DATABASE_URL"
  PGHOST=localhost PGPORT=5432 PGUSER=postgres PGPASSWORD=postgres pgsize

Keys:
  enter / l / right    drill in
  backspace / h / left go back
  j / down             move down
  k / up               move up
  g / home             jump to top
  G / end              jump to bottom
  s                    toggle sort (size / name)
  /                    filter
  r                    reload
  q / ctrl+c           quit

Flags:
`

func main() {
	dsn := flag.String("dsn", "", "Postgres connection string; if empty, PG* env vars/libpq defaults are used")
	showVer := flag.Bool("version", false, "print version and exit")

	flag.Usage = func() {
		fmtx.Fprint(os.Stderr, usage)
		flag.PrintDefaults()
		fmtx.Fprintln(os.Stderr)
	}
	flag.Parse()

	if *showVer {
		fmt.Printf("%s\n", Version)
		os.Exit(0)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *dsn)
	if err != nil {
		fmtx.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	if err := pool.Ping(ctx); err != nil {
		fmtx.Fprintf(os.Stderr, "ping: %v\n", err)
		pool.Close()
		os.Exit(1)
	}

	dbs, err := pg.ListDatabases(ctx, pool)
	if err != nil {
		fmtx.Fprintf(os.Stderr, "list databases: %v\n", err)
		pool.Close()
		os.Exit(1)
	}

	app := ui.InitialModel(pool, dbs, *dsn)
	p := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmtx.Fprintf(os.Stderr, "tui: %v\n", err)
		pool.Close()
		os.Exit(1)
	}
}
