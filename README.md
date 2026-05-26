# pgsize

Fast size viewer for PostgreSQL cluster (ncdu for pg).

[![License](https://img.shields.io/github/license/hashmap-kz/pgsize)](https://github.com/hashmap-kz/pgsize/blob/master/LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/hashmap-kz/pgsize)](https://goreportcard.com/report/github.com/hashmap-kz/pgsize)
[![Go Reference](https://pkg.go.dev/badge/github.com/hashmap-kz/pgsize.svg)](https://pkg.go.dev/github.com/hashmap-kz/pgsize)
[![Workflow Status](https://img.shields.io/github/actions/workflow/status/hashmap-kz/pgsize/ci.yml?branch=master)](https://github.com/hashmap-kz/pgsize/actions/workflows/ci.yml?query=branch:master)
[![Go Version](https://img.shields.io/github/go-mod/go-version/hashmap-kz/pgsize)](https://github.com/hashmap-kz/pgsize/blob/master/go.mod#L3)
[![Latest Release](https://img.shields.io/github/v/release/hashmap-kz/pgsize)](https://github.com/hashmap-kz/pgsize/releases/latest)

![Preview](https://raw.githubusercontent.com/hashmap-kz/assets/main/pgsize/pgsize-2.gif)

---

## Install

**Using Go:**

```bash
go install github.com/hashmap-kz/pgsize@latest
```

**Using Homebrew:**

```bash
brew tap hashmap-kz/homebrew-tap
brew install pgsize
```

Or download a binary from the [Releases page](https://github.com/hashmap-kz/pgsize/releases).

---

## Usage

```bash
# Using CLI flags
pgsize -dsn "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"

# Using PG* env vars / libpq-style defaults
PGHOST=localhost PGPORT=5432 PGUSER=postgres PGPASSWORD=postgres PGDATABASE=postgres \
    pgsize
```

---

## License

MIT. See [LICENSE](./LICENSE) for details.
