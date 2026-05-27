SELECT c.oid :: int8                                                          AS id,
       nc.nspname                                                             AS schema,
       c.relname                                                              AS name,
       pg_total_relation_size(format('%I.%I', nc.nspname, c.relname)) :: int8 AS bytes,
       pg_size_pretty(
               pg_total_relation_size(format('%I.%I', nc.nspname, c.relname))
       )                                                                      AS size,
       pg_stat_get_live_tuples(c.oid)                                         AS live_rows_estimate,
       pg_stat_get_dead_tuples(c.oid)                                         AS dead_rows_estimate,
       obj_description(c.oid)                                                 AS comment
FROM pg_namespace nc
         JOIN pg_class c ON nc.oid = c.relnamespace
WHERE c.relkind IN ('r', 'p')
  AND NOT pg_is_other_temp_schema(nc.oid)
  AND (
    pg_has_role(c.relowner, 'USAGE')
        OR has_table_privilege(
            c.oid,
            'SELECT, INSERT, UPDATE, DELETE, TRUNCATE, REFERENCES, TRIGGER'
           )
        OR has_any_column_privilege(c.oid, 'SELECT, INSERT, UPDATE, REFERENCES')
    )
group by c.oid,
         c.relname,
         nc.nspname
;

-- https://www.postgresql.org/docs/current/catalog-pg-class.html
--
-- relkind char
--
-- r = ordinary table,
-- i = index,
-- S = sequence,
-- t = TOAST table,
-- v = view,
-- m = materialized view,
-- c = composite type,
-- f = foreign table,
-- p = partitioned table,
-- I = partitioned index

-- c.relkind in ('v', 'r', 'm', 'f')
