// pull-levels downloads every level JSON from the Neon DB and writes each
// row to internal/game/levels/<name>, overwriting the local file. Useful to
// bring admin-edited levels back into version control.
//
// Usage:
//   DATABASE_URL=postgres://... go run ./cmd/pull-levels [-out <dir>]
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"
)

func main() {
	out := flag.String("out", "internal/game/levels", "destination directory for pulled .json files")
	flag.Parse()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL not set")
		os.Exit(1)
	}

	if err := os.MkdirAll(*out, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir %s: %v\n", *out, err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close(ctx)

	rows, err := conn.Query(ctx, `SELECT name, definition FROM levels ORDER BY name`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "query: %v\n", err)
		os.Exit(1)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var name string
		var def []byte
		if err := rows.Scan(&name, &def); err != nil {
			fmt.Fprintf(os.Stderr, "scan: %v\n", err)
			os.Exit(1)
		}
		pretty, err := prettify(def)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: invalid JSON from DB: %v\n", name, err)
			os.Exit(1)
		}
		path := filepath.Join(*out, name)
		if err := os.WriteFile(path, pretty, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", path, err)
			os.Exit(1)
		}
		fmt.Printf("pulled %s (%d bytes)\n", path, len(pretty))
		count++
	}
	if err := rows.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "rows: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("done: %d level(s) written to %s\n", count, *out)
}

func prettify(raw []byte) ([]byte, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}
