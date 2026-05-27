# pgsize

Fast size viewer for PostgreSQL clusters (ncdu for pg).

[![License](https://img.shields.io/github/license/hashmap-kz/pgsize)](https://github.com/hashmap-kz/pgsize/blob/master/LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/hashmap-kz/pgsize)](https://goreportcard.com/report/github.com/hashmap-kz/pgsize)
[![Go Reference](https://pkg.go.dev/badge/github.com/hashmap-kz/pgsize.svg)](https://pkg.go.dev/github.com/hashmap-kz/pgsize)
[![Workflow Status](https://img.shields.io/github/actions/workflow/status/hashmap-kz/pgsize/ci.yml?branch=master)](https://github.com/hashmap-kz/pgsize/actions/workflows/ci.yml?query=branch:master)
[![Go Version](https://img.shields.io/github/go-mod/go-version/hashmap-kz/pgsize)](https://github.com/hashmap-kz/pgsize/blob/master/go.mod#L3)
[![Latest Release](https://img.shields.io/github/v/release/hashmap-kz/pgsize)](https://github.com/hashmap-kz/pgsize/releases/latest)

![Preview](https://raw.githubusercontent.com/hashmap-kz/assets/main/pgsize/pgsize-5.gif)

---

## Install

**Using installation script**

```bash
curl -fsSL https://raw.githubusercontent.com/hashmap-kz/pgsize/master/scripts/install.sh | sh
```

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
pgsize -dsn "host=localhost port=5432 user=postgres password=$mypasswd"

# Using PG* env vars / libpq-style defaults
PGHOST=localhost PGPORT=5432 PGUSER=postgres PGPASSWORD=postgres PGDATABASE=postgres \
    pgsize
    
# Connect to multiple clusters
pgsize \
  --dsn "postgres://user:pass@dev:5432/db" \
  --dsn "postgres://user:pass@stage:5432/db"
```

---

## License

MIT. See [LICENSE](./LICENSE) for details.
