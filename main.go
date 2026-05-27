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
  r                    reload
  q / ctrl+c           quit

Flags:
`

type runOpts struct {
	dsn     string
	noColor bool
}

func main() {
	dsn := flag.String("dsn", "", "Postgres connection string; if empty, PG* env vars/libpq defaults are used")
	showVer := flag.Bool("version", false, "print version and exit")
	noColor := flag.Bool("no-color", false, "disable colors and text styles (also honoured via NO_COLOR env var)")

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

	opts := &runOpts{
		dsn:     *dsn,
		noColor: *noColor || isNoColor(),
	}
	if err := run(opts); err != nil {
		fmtx.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func run(o *runOpts) error {
	if o.noColor {
		ui.DisableStyles()
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, o.dsn)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping: %w", err)
	}

	dbs, err := pg.ListDatabases(ctx, pool)
	if err != nil {
		return fmt.Errorf("list databases: %w", err)
	}

	app := ui.InitialModel(pool, dbs, o.dsn)
	p := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	return nil
}

func isNoColor() bool {
	_, set := os.LookupEnv("NO_COLOR")
	return set
}
