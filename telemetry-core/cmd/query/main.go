package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	_ "github.com/marcboeker/go-duckdb"
)

func main() {
	tables := flag.Bool("tables", false, "List all tables with row counts")
	schema := flag.Bool("schema", false, "Show full schema for all tables")
	flag.Parse()

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "workspace/telemetry.duckdb"
	}

	dsn := dbPath + "?access_mode=READ_ONLY"
	db, err := sql.Open("duckdb", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot open database %s: %v\n", dbPath, err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot connect to database %s: %v\n", dbPath, err)
		os.Exit(1)
	}

	switch {
	case *tables:
		runTables(db)
	case *schema:
		runSchema(db)
	default:
		args := flag.Args()
		if len(args) == 0 {
			fmt.Fprintln(os.Stderr, "usage: racedb [--tables | --schema] \"SQL QUERY\"")
			os.Exit(1)
		}
		runQuery(db, args[0])
	}
}

func runTables(db *sql.DB) {
	rows, err := db.Query(`
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = 'main'
		ORDER BY table_name
	`)
	if err != nil {
		fatal("query tables: %v", err)
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			fatal("scan table name: %v", err)
		}
		var count int64
		err := db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", name)).Scan(&count)
		if err != nil {
			fatal("count %s: %v", name, err)
		}
		results = append(results, map[string]interface{}{
			"table": name,
			"rows":  count,
		})
	}
	printJSON(results)
}

func runSchema(db *sql.DB) {
	rows, err := db.Query(`
		SELECT table_name, column_name, data_type
		FROM information_schema.columns
		WHERE table_schema = 'main'
		ORDER BY table_name, ordinal_position
	`)
	if err != nil {
		fatal("query schema: %v", err)
	}
	defer rows.Close()

	var results []map[string]string
	for rows.Next() {
		var table, col, dtype string
		if err := rows.Scan(&table, &col, &dtype); err != nil {
			fatal("scan schema: %v", err)
		}
		results = append(results, map[string]string{
			"table":  table,
			"column": col,
			"type":   dtype,
		})
	}
	printJSON(results)
}

func runQuery(db *sql.DB, query string) {
	rows, err := db.Query(query)
	if err != nil {
		fatal("query error: %v", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		fatal("columns error: %v", err)
	}

	var results []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			fatal("scan error: %v", err)
		}
		row := make(map[string]interface{})
		for i, col := range cols {
			val := values[i]
			if b, ok := val.([]byte); ok {
				val = string(b)
			}
			row[col] = val
		}
		results = append(results, row)
	}

	if results == nil {
		results = []map[string]interface{}{}
	}
	printJSON(results)
}

func printJSON(v interface{}) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fatal("json encode: %v", err)
	}
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
