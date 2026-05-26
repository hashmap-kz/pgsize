package pg

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Database struct {
	Name      string
	SizeBytes uint64
}

type Schema struct {
	Name       string
	SizeBytes  uint64
	TableCount uint32
	IndexCount uint32
}

type Index struct {
	Name      string
	SizeBytes uint64
}

type Table struct {
	Schema     string
	Name       string
	TotalBytes uint64
	Indexes    []Index
}

type RelKind int

const (
	RelUnknown RelKind = iota
	RelHeap
	RelToast
	RelFsmVm
	RelBtree
	RelGin
	RelGist
	RelHash
	RelBrin
	RelSpgist
)

func (k RelKind) String() string {
	switch k {
	case RelUnknown:
		return "UNKNOWN"
	case RelHeap:
		return "HEAP"
	case RelToast:
		return "TOAST"
	case RelFsmVm:
		return "FSM/VM"
	case RelBtree:
		return "BTREE"
	case RelGin:
		return "GIN"
	case RelGist:
		return "GIST"
	case RelHash:
		return "HASH"
	case RelBrin:
		return "BRIN"
	case RelSpgist:
		return "SPGIST"
	}
	return "UNKNOWN"
}

func (k RelKind) IsIndex() bool {
	switch k {
	case RelHeap, RelToast, RelFsmVm:
		return false
	default:
		return true
	}
}

type Relation struct {
	Name      string
	Kind      RelKind
	SizeBytes uint64
	BloatPct  float64
}

func ListDatabases(ctx context.Context, pool *pgxpool.Pool) ([]Database, error) {
	const q = `
		SELECT
		    d.datname,
		    pg_database_size(d.oid)::bigint AS size_bytes
		FROM pg_database d
		WHERE NOT d.datistemplate
		  AND has_database_privilege(d.datname, 'CONNECT')
		ORDER BY size_bytes DESC
	`
	rows, err := pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Database
	for rows.Next() {
		var d Database
		var size int64
		if err := rows.Scan(&d.Name, &size); err != nil {
			return nil, err
		}
		d.SizeBytes = uint64(size)
		out = append(out, d)
	}
	return out, rows.Err()
}

func ListSchemas(ctx context.Context, pool *pgxpool.Pool) ([]Schema, error) {
	const q = `
		SELECT
		    n.nspname,
		    COALESCE(SUM(pg_total_relation_size(c.oid)) FILTER (WHERE c.relkind IN ('r','p','m')), 0)::bigint AS size_bytes,
		    COUNT(*) FILTER (WHERE c.relkind IN ('r','p','m'))::int AS table_count,
		    COUNT(*) FILTER (WHERE c.relkind IN ('i','I'))::int     AS index_count
		FROM pg_namespace n
		LEFT JOIN pg_class c ON c.relnamespace = n.oid
		WHERE n.nspname NOT IN ('pg_catalog','information_schema','pg_toast')
		  AND n.nspname NOT LIKE 'pg_temp_%'
		  AND n.nspname NOT LIKE 'pg_toast_temp_%'
		GROUP BY n.nspname
		ORDER BY size_bytes DESC
	`
	rows, err := pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Schema
	for rows.Next() {
		var s Schema
		var size int64
		var tcount, icount int32
		if err := rows.Scan(&s.Name, &size, &tcount, &icount); err != nil {
			return nil, err
		}
		s.SizeBytes = uint64(size)
		s.TableCount = uint32(tcount)
		s.IndexCount = uint32(icount)
		out = append(out, s)
	}
	return out, rows.Err()
}

func ListTables(ctx context.Context, pool *pgxpool.Pool, schema string) ([]Table, error) {
	const q = `
		SELECT
		    n.nspname,
		    c.relname,
		    pg_total_relation_size(c.oid)::bigint AS total,
		    i.relname                             AS idx_name,
		    pg_relation_size(i.oid)::bigint       AS idx_size
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		LEFT JOIN pg_index x ON x.indrelid = c.oid
		LEFT JOIN pg_class i ON i.oid = x.indexrelid
		WHERE c.relkind IN ('r','p')
		  AND n.nspname = $1
		ORDER BY total DESC, c.oid, idx_size DESC NULLS LAST
	`
	rows, err := pool.Query(ctx, q, schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var order []string
	tables := make(map[string]*Table)

	for rows.Next() {
		var schName, name string
		var total int64
		var idxName *string
		var idxSize *int64
		if err := rows.Scan(&schName, &name, &total, &idxName, &idxSize); err != nil {
			return nil, err
		}
		t, ok := tables[name]
		if !ok {
			order = append(order, name)
			tables[name] = &Table{Schema: schName, Name: name, TotalBytes: uint64(total)}
			t = tables[name]
		}
		if idxName != nil && idxSize != nil {
			t.Indexes = append(t.Indexes, Index{Name: *idxName, SizeBytes: uint64(*idxSize)})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]Table, len(order))
	for i, name := range order {
		out[i] = *tables[name]
	}
	return out, nil
}

func ListRelations(ctx context.Context, pool *pgxpool.Pool, schema, table string) ([]Relation, error) {
	const q = `
		WITH t AS (
		    SELECT c.oid, c.reltoastrelid
		    FROM pg_class c
		    JOIN pg_namespace n ON n.oid = c.relnamespace
		    WHERE n.nspname = $1 AND c.relname = $2
		)
		-- heap
		SELECT 'table data'::text AS name,
		       'HEAP'::text       AS kind,
		       pg_table_size(t.oid)::bigint AS size,
		       0 AS sort_group
		    FROM t
		UNION ALL
		-- toast (only if present)
		SELECT 'toast'::text,
		       'TOAST'::text,
		       pg_total_relation_size(t.reltoastrelid)::bigint,
		       1
		    FROM t WHERE t.reltoastrelid <> 0
		UNION ALL
		-- user indexes only (exclude toast indexes)
		SELECT i.relname,
		       UPPER(am.amname),
		       pg_relation_size(i.oid)::bigint,
		       2
		    FROM t
		    JOIN pg_index x ON x.indrelid = t.oid
		    JOIN pg_class i ON i.oid = x.indexrelid
		    JOIN pg_am    am ON am.oid = i.relam
		ORDER BY sort_group, size DESC
	`
	rows, err := pool.Query(ctx, q, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Relation
	for rows.Next() {
		var r Relation
		var kind string
		var size int64
		var grp int
		if err := rows.Scan(&r.Name, &kind, &size, &grp); err != nil {
			return nil, err
		}
		r.SizeBytes = uint64(size)
		switch kind {
		case "HEAP":
			r.Kind = RelHeap
		case "TOAST":
			r.Kind = RelToast
		case "BTREE":
			r.Kind = RelBtree
		case "GIN":
			r.Kind = RelGin
		case "GIST":
			r.Kind = RelGist
		case "HASH":
			r.Kind = RelHash
		case "BRIN":
			r.Kind = RelBrin
		case "SPGIST":
			r.Kind = RelSpgist
		default:
			r.Kind = RelUnknown
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
