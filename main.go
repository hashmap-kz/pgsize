package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/hashmap-kz/pgsize/internal/ui"
	"github.com/hashmap-kz/pgsize/internal/x/fmtx"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jackc/pgx/v5/pgxpool"
)

var Version = "dev"

//nolint:gosec // false positive: example URL in usage text, not real credentials
var usage = `pgsize - interactive TUI for exploring PostgreSQL database sizes

Browse databases, schemas, tables, and index sizes in a terminal UI.
Pass --dsn once for a single cluster, or repeat it for multi-cluster mode.
With no --dsn flag, libpq environment variables (PGHOST, PGUSER, etc.) are used.

Usage:
  pgsize [flags]

Examples:
  pgsize
  pgsize --dsn "postgres://user:pass@host:port/db"
  pgsize --dsn "host=localhost port=5432 user=postgres password=$mypasswd"
  pgsize --dsn "postgres://user:pass@dev:5432/db" --dsn "postgres://user:pass@stage:5432/db"
  PGHOST=localhost PGPORT=5432 PGUSER=postgres PGPASSWORD=postgres pgsize

Keys:
  enter / l / right    drill in / select cluster
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

type multiFlag []string

func (f *multiFlag) String() string     { return strings.Join(*f, ", ") }
func (f *multiFlag) Set(v string) error { *f = append(*f, v); return nil }

type runOpts struct {
	dsns    []string
	noColor bool
}

func main() {
	var dsns multiFlag
	showVer := flag.Bool("version", false, "print version and exit")
	noColor := flag.Bool("no-color", false, "disable colors and text styles (also honoured via NO_COLOR env var)")
	flag.Var(&dsns, "dsn", "Postgres connection string (may be repeated for multiple clusters; if omitted, libpq env vars are used)")

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

	if len(dsns) == 0 {
		dsns = []string{""}
	}

	opts := &runOpts{
		dsns:    dsns,
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
	clusters := make([]ui.Cluster, 0, len(o.dsns))
	for _, dsn := range o.dsns {
		pool, err := pgxpool.New(ctx, dsn)
		if err != nil {
			return fmt.Errorf("connect %s: %w", clusterName(dsn), err)
		}
		if err := pool.Ping(ctx); err != nil {
			pool.Close()
			return fmt.Errorf("ping %s: %w", clusterName(dsn), err)
		}
		clusters = append(clusters, ui.Cluster{
			Name: clusterName(dsn),
			DSN:  dsn,
			Pool: pool,
		})
	}
	defer func() {
		for _, c := range clusters {
			c.Pool.Close()
		}
	}()

	app := ui.InitialModel(clusters)
	p := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	return nil
}

func clusterName(dsn string) string {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil || cfg.ConnConfig.Host == "" {
		if dsn == "" {
			return "localhost"
		}
		return dsn
	}
	port := cfg.ConnConfig.Port
	if port == 0 || port == 5432 {
		return cfg.ConnConfig.Host
	}
	return fmt.Sprintf("%s:%d", cfg.ConnConfig.Host, int(port))
}

func isNoColor() bool {
	_, set := os.LookupEnv("NO_COLOR")
	return set
}
