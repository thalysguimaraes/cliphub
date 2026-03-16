package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

// MaxContentSize is the maximum allowed clipboard content size (10MB).
const MaxContentSize = 10 << 20

// ClipItem is the canonical representation of a clipboard entry.
type ClipItem struct {
	Seq       uint64    `json:"seq"`
	MimeType  string    `json:"mime_type"`
	Content   string    `json:"content,omitempty"`   // Text content (text/* types).
	Data      []byte    `json:"data,omitempty"`      // Binary content (base64 in JSON).
	Hash      string    `json:"hash"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// IsText returns whether this item is a text type.
func (c *ClipItem) IsText() bool {
	return strings.HasPrefix(c.MimeType, "text/")
}

// RawBytes returns the raw content bytes regardless of type.
func (c *ClipItem) RawBytes() []byte {
	if c.IsText() {
		return []byte(c.Content)
	}
	return c.Data
}

// WSMessage wraps messages sent over the WebSocket connection.
type WSMessage struct {
	Type string    `json:"type"`
	Item *ClipItem `json:"item,omitempty"`
}

// HashBytes computes the SHA-256 hex digest of data.
func HashBytes(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// HashContent computes the SHA-256 hex digest of a string.
func HashContent(s string) string {
	return HashBytes([]byte(s))
}
