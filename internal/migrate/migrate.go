// Package migrate applies the embedded baseline schema. Migrations are a list
// of *.up.sql files ordered by version; each is applied once and recorded in
// schema_migrations. The runner holds a Postgres advisory lock for the run, so
// two API processes starting together cannot apply the same file twice.

package migrate

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed *.up.sql
var files embed.FS

type pendingMigration struct {
	version  int
	fileName string
	script   string
}

// load returns every embedded migration in ascending version order.
func load() ([]pendingMigration, error) {
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}

	pending := make([]pendingMigration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".up.sql") {
			continue
		}
		version, err := strconv.Atoi(strings.TrimSuffix(entry.Name(), ".up.sql"))
		if err != nil {
			return nil, fmt.Errorf("parse migration version from %s: %w", entry.Name(), err)
		}
		script, err := files.ReadFile(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		pending = append(pending, pendingMigration{
			version:  version,
			fileName: entry.Name(),
			script:   string(script),
		})
	}

	sort.Slice(pending, func(i, j int) bool {
		return pending[i].version < pending[j].version
	})
	return pending, nil
}

// Apply runs every not-yet-applied migration in version order inside one
// transaction, holding an advisory lock so concurrent starters cannot double
// apply. It is safe to call on every boot.
func Apply(ctx context.Context, pool *pgxpool.Pool) error {
	return apply(ctx, pool, 0)
}

// apply is Apply with a version ceiling; 0 means no ceiling.
//
// The ceiling exists for the per-migration tests. A test named after 0008 or
// 0010 asserts the schema *that file* leaves behind, and a later file can undo
// part of it — 0011 drops the columns 0010 deliberately kept — so an uncapped
// run makes those assertions describe a schema their file never produced.
// Production never passes a ceiling: every migration runs.
func apply(ctx context.Context, pool *pgxpool.Pool, maxVersion int) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Release()

	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin migration transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const lockID = 0x1A6E6C44 // advisory key; serialises migration runners
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", lockID); err != nil {
		return fmt.Errorf("acquire advisory lock: %w", err)
	}

	if _, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	applied, err := appliedVersions(ctx, tx)
	if err != nil {
		return err
	}

	pending, err := load()
	if err != nil {
		return err
	}

	for _, migration := range pending {
		if applied[migration.version] {
			continue
		}
		if maxVersion > 0 && migration.version > maxVersion {
			continue
		}
		for _, statement := range statements(migration.script) {
			if _, err := tx.Exec(ctx, statement); err != nil {
				return fmt.Errorf("apply %s: %w", migration.fileName, err)
			}
		}
		if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", migration.version); err != nil {
			return fmt.Errorf("record %s: %w", migration.fileName, err)
		}
	}

	return tx.Commit(ctx)
}

func appliedVersions(ctx context.Context, tx pgx.Tx) (map[int]bool, error) {
	rows, err := tx.Query(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("read applied migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[int]bool)
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("scan applied migration: %w", err)
		}
		applied[version] = true
	}
	return applied, rows.Err()
}

// statements splits SQL on semicolons that sit outside a comment, a
// single-quoted literal, or a dollar-quoted string. A naive split on ";" would
// break the DO $$ ... $$ role block and the comments in the baseline file.
func statements(script string) []string {
	var split []string
	var builder strings.Builder

	flush := func() {
		statement := strings.TrimSpace(builder.String())
		if statement != "" {
			split = append(split, statement)
		}
		builder.Reset()
	}

	for index := 0; index < len(script); index++ {
		switch char := script[index]; char {
		case '-':
			if index+1 < len(script) && script[index+1] == '-' {
				// Line comment: keep it verbatim, but ignore any ';' inside.
				end := strings.IndexByte(script[index:], '\n')
				if end < 0 {
					builder.WriteString(script[index:])
					index = len(script)
				} else {
					builder.WriteString(script[index : index+end])
					index += end - 1
				}
				continue
			}
			builder.WriteByte(char)
		case '/':
			if index+1 < len(script) && script[index+1] == '*' {
				// Block comment: keep it verbatim, but ignore any ';' inside.
				end := strings.Index(script[index:], "*/")
				if end < 0 {
					builder.WriteString(script[index:])
					index = len(script)
				} else {
					builder.WriteString(script[index : index+end+2])
					index += end + 1
				}
				continue
			}
			builder.WriteByte(char)
		case '\'':
			// Single-quoted literal: '' is an escaped quote, not the end.
			builder.WriteByte(char)
			for index++; index < len(script); index++ {
				builder.WriteByte(script[index])
				if script[index] != '\'' {
					continue
				}
				if index+1 < len(script) && script[index+1] == '\'' {
					builder.WriteByte(script[index+1])
					index++
					continue
				}
				break
			}
		case '$':
			end := findDollarQuoteEnd(script, index)
			if end < 0 {
				// A lone '$' (e.g. inside a numeric constant) is literal.
				builder.WriteByte(char)
				continue
			}
			builder.WriteString(script[index : end+1])
			index = end
		case ';':
			flush()
		default:
			builder.WriteByte(char)
		}
	}

	flush()
	return split
}

// findDollarQuoteEnd returns the index of the closing '$' of the dollar-quoted
// string opening at openIndex, or -1 when that '$' does not open one. A tag is
// optional: $$...$$ and $tag$...$tag$ are both valid.
func findDollarQuoteEnd(script string, openIndex int) int {
	close := openIndex + 1
	for close < len(script) && isDollarTagByte(script[close]) {
		close++
	}
	if close == len(script) || script[close] != '$' {
		return -1
	}

	tag := script[openIndex : close+1]
	rest := script[close+1:]
	if position := strings.Index(rest, tag); position >= 0 {
		return close + position + len(tag) - 1
	}

	// Unterminated dollar quote: hand the rest of the script back whole.
	return len(script) - 1
}

func isDollarTagByte(char byte) bool {
	return char >= 'a' && char <= 'z' ||
		char >= 'A' && char <= 'Z' ||
		char >= '0' && char <= '9' ||
		char == '_'
}
