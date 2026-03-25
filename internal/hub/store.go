package hub

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"github.com/thalysguimaraes/cliphub/internal/protocol"
)

// Store provides durable persistence for hub state.
type Store struct {
	db *sql.DB
}

// OpenStore opens or creates a SQLite database at path.
func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable WAL: %w", err)
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS clips (
			seq        INTEGER PRIMARY KEY,
			mime_type  TEXT    NOT NULL,
			content    TEXT,
			data       BLOB,
			hash       TEXT    NOT NULL,
			source     TEXT    NOT NULL,
			created_at TEXT    NOT NULL,
			expires_at TEXT    NOT NULL
		);
		CREATE TABLE IF NOT EXISTS meta (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
	`)
	return err
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

// LoadState restores the seq counter and up to maxHistory items from the DB.
// Items are returned newest-first.
func (s *Store) LoadState(maxHistory int) (uint64, []protocol.ClipItem, error) {
	var seq uint64
	row := s.db.QueryRow("SELECT value FROM meta WHERE key = 'seq'")
	if err := row.Scan(&seq); err != nil && err != sql.ErrNoRows {
		return 0, nil, fmt.Errorf("load seq: %w", err)
	}

	rows, err := s.db.Query(
		"SELECT seq, mime_type, content, data, hash, source, created_at, expires_at FROM clips ORDER BY seq DESC LIMIT ?",
		maxHistory,
	)
	if err != nil {
		return seq, nil, fmt.Errorf("load history: %w", err)
	}
	defer rows.Close()

	var items []protocol.ClipItem
	for rows.Next() {
		var item protocol.ClipItem
		var content sql.NullString
		var data []byte
		var createdAt, expiresAt string

		if err := rows.Scan(&item.Seq, &item.MimeType, &content, &data, &item.Hash, &item.Source, &createdAt, &expiresAt); err != nil {
			return seq, nil, fmt.Errorf("scan clip: %w", err)
		}
		item.Content = content.String
		item.Data = data
		item.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		item.ExpiresAt, _ = time.Parse(time.RFC3339Nano, expiresAt)
		items = append(items, item)
	}
	return seq, items, rows.Err()
}

// SaveItem persists a clip item.
func (s *Store) SaveItem(item protocol.ClipItem) error {
	_, err := s.db.Exec(
		"INSERT OR REPLACE INTO clips (seq, mime_type, content, data, hash, source, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		item.Seq, item.MimeType, item.Content, item.Data, item.Hash, item.Source,
		item.CreatedAt.Format(time.RFC3339Nano), item.ExpiresAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("INSERT OR REPLACE INTO meta (key, value) VALUES ('seq', ?)", item.Seq)
	return err
}

// DeleteExpired removes clips that have expired before the given time.
func (s *Store) DeleteExpired(before time.Time) (int, error) {
	result, err := s.db.Exec("DELETE FROM clips WHERE expires_at < ?", before.Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}
