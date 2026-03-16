package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// MaxContentSize is the maximum allowed clipboard content size (1MB).
const MaxContentSize = 1 << 20

// ClipItem is the canonical representation of a clipboard entry.
type ClipItem struct {
	Seq       uint64    `json:"seq"`
	Content   string    `json:"content"`
	Hash      string    `json:"hash"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// WSMessage wraps messages sent over the WebSocket connection.
type WSMessage struct {
	Type string    `json:"type"`
	Item *ClipItem `json:"item,omitempty"`
}

// HashContent computes the SHA-256 hex digest of s.
func HashContent(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
