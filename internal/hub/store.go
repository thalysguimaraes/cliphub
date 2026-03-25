package hub

import (
	"database/sql"
	"fmt"
	"strings"
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
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin migration: %w", err)
	}
	defer tx.Rollback()

	var schema string
	err = tx.QueryRow("SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'clips'").Scan(&schema)
	switch {
	case err == sql.ErrNoRows:
		if _, err := tx.Exec(clipsTableDDL); err != nil {
			return fmt.Errorf("create clips table: %w", err)
		}
	case err != nil:
		return fmt.Errorf("inspect clips schema: %w", err)
	case !strings.Contains(strings.ToUpper(schema), "AUTOINCREMENT"):
		if err := migrateLegacyClipsTable(tx); err != nil {
			return err
		}
	}

	if _, err := tx.Exec("DROP TABLE IF EXISTS meta"); err != nil {
		return fmt.Errorf("drop legacy meta table: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration: %w", err)
	}
	return nil
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

// LoadState restores the seq counter and up to maxHistory items from the DB.
// Items are returned newest-first.
func (s *Store) LoadState(maxHistory int) (uint64, []protocol.ClipItem, error) {
	seq, err := s.loadSeq()
	if err != nil {
		return 0, nil, err
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

// SaveItem persists a clip item and returns it with seq assigned.
func (s *Store) SaveItem(item protocol.ClipItem) (protocol.ClipItem, error) {
	if item.Seq == 0 {
		result, err := s.db.Exec(
			"INSERT INTO clips (mime_type, content, data, hash, source, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
			item.MimeType, item.Content, item.Data, item.Hash, item.Source,
			item.CreatedAt.Format(time.RFC3339Nano), item.ExpiresAt.Format(time.RFC3339Nano),
		)
		if err != nil {
			return item, err
		}
		seq, err := result.LastInsertId()
		if err != nil {
			return item, fmt.Errorf("read inserted seq: %w", err)
		}
		item.Seq = uint64(seq)
		return item, nil
	}

	_, err := s.db.Exec(
		"INSERT OR REPLACE INTO clips (seq, mime_type, content, data, hash, source, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		item.Seq, item.MimeType, item.Content, item.Data, item.Hash, item.Source,
		item.CreatedAt.Format(time.RFC3339Nano), item.ExpiresAt.Format(time.RFC3339Nano),
	)
	return item, err
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

const clipsTableDDL = `
	CREATE TABLE clips (
		seq        INTEGER PRIMARY KEY AUTOINCREMENT,
		mime_type  TEXT    NOT NULL,
		content    TEXT,
		data       BLOB,
		hash       TEXT    NOT NULL,
		source     TEXT    NOT NULL,
		created_at TEXT    NOT NULL,
		expires_at TEXT    NOT NULL
	);
`

func migrateLegacyClipsTable(tx *sql.Tx) error {
	if _, err := tx.Exec("ALTER TABLE clips RENAME TO clips_legacy"); err != nil {
		return fmt.Errorf("rename legacy clips table: %w", err)
	}
	if _, err := tx.Exec(clipsTableDDL); err != nil {
		return fmt.Errorf("create migrated clips table: %w", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO clips (seq, mime_type, content, data, hash, source, created_at, expires_at)
		SELECT seq, mime_type, content, data, hash, source, created_at, expires_at
		FROM clips_legacy
		ORDER BY seq
	`); err != nil {
		return fmt.Errorf("copy legacy clips rows: %w", err)
	}
	if _, err := tx.Exec("DROP TABLE clips_legacy"); err != nil {
		return fmt.Errorf("drop legacy clips table: %w", err)
	}
	return nil
}

func (s *Store) loadSeq() (uint64, error) {
	var seq uint64
	row := s.db.QueryRow("SELECT seq FROM sqlite_sequence WHERE name = 'clips'")
	if err := row.Scan(&seq); err != nil {
		if err != sql.ErrNoRows {
			return 0, fmt.Errorf("load seq from sqlite_sequence: %w", err)
		}

		row = s.db.QueryRow("SELECT COALESCE(MAX(seq), 0) FROM clips")
		if err := row.Scan(&seq); err != nil {
			return 0, fmt.Errorf("load seq from clips: %w", err)
		}
	}
	return seq, nil
}
