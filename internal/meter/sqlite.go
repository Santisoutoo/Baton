// Package meter records per-request token usage locally so `batuta usage` can
// report spend without depending on any provider's usage API. Backed by a pure-Go
// SQLite (no cgo) that lives next to the config file.
package meter

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"batuta/internal/core"

	_ "modernc.org/sqlite"
)

// SQLite implements core.MeterStore.
type SQLite struct {
	db *sql.DB
}

// Open opens (creating if needed) the usage database at path.
func Open(path string) (*SQLite, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS usage (
			ts          INTEGER NOT NULL,
			backend     TEXT    NOT NULL,
			model       TEXT    NOT NULL,
			role        TEXT    NOT NULL,
			in_tokens   INTEGER NOT NULL,
			out_tokens  INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_usage_ts ON usage(ts);
	`); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &SQLite{db: db}, nil
}

// Record appends one usage row. Timestamp defaults to now if unset.
func (s *SQLite) Record(u core.Usage) error {
	ts := u.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	_, err := s.db.Exec(
		`INSERT INTO usage (ts, backend, model, role, in_tokens, out_tokens) VALUES (?,?,?,?,?,?)`,
		ts.Unix(), u.Backend, u.Model, u.Role, u.InputTokens, u.OutputTokens,
	)
	return err
}

// Query aggregates usage since f.Since, grouped by f.By (model|backend|day).
func (s *SQLite) Query(f core.Filter) ([]core.Aggregate, error) {
	keyExpr := "model"
	switch f.By {
	case "backend":
		keyExpr = "backend"
	case "day":
		keyExpr = "date(ts, 'unixepoch')"
	case "", "model":
		keyExpr = "model"
	default:
		return nil, fmt.Errorf("unknown group-by %q (want model|backend|day)", f.By)
	}

	since := int64(0)
	if !f.Since.IsZero() {
		since = f.Since.Unix()
	}
	q := fmt.Sprintf(`
		SELECT %s AS k, COUNT(*), COALESCE(SUM(in_tokens),0), COALESCE(SUM(out_tokens),0)
		FROM usage WHERE ts >= ?
		GROUP BY k ORDER BY SUM(in_tokens)+SUM(out_tokens) DESC`, keyExpr)

	rows, err := s.db.Query(q, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []core.Aggregate
	for rows.Next() {
		var a core.Aggregate
		if err := rows.Scan(&a.Key, &a.Requests, &a.InputTokens, &a.OutputTokens); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Close releases the database.
func (s *SQLite) Close() error { return s.db.Close() }
