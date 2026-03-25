package protocol

import (
	"fmt"
	"time"
)

const (
	// DefaultHistoryLimit is the default number of history items returned by paged history APIs.
	DefaultHistoryLimit = 50
	// MaxHistoryPageLimit caps individual history page sizes so clients can't request unbounded pages.
	MaxHistoryPageLimit = 200
)

// APIError is the structured error payload returned by HTTP handlers.
type APIError struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"`
}

// ErrorResponse wraps structured API errors.
type ErrorResponse struct {
	Error APIError `json:"error"`
}

// ClipSummary is the lightweight API representation used by paged history and blob flows.
type ClipSummary struct {
	Seq          uint64    `json:"seq"`
	MimeType     string    `json:"mime_type"`
	Content      string    `json:"content,omitempty"`
	Hash         string    `json:"hash"`
	Source       string    `json:"source"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	SizeBytes    int       `json:"size_bytes"`
	DownloadPath string    `json:"download_path,omitempty"`
}

// HistoryPage is the cursor-based history response envelope.
type HistoryPage struct {
	Items      []ClipSummary `json:"items"`
	NextCursor string        `json:"next_cursor,omitempty"`
	HasMore    bool          `json:"has_more"`
}

// SummarizeClip converts a stored clip item into its lightweight API representation.
func SummarizeClip(item ClipItem) ClipSummary {
	summary := ClipSummary{
		Seq:          item.Seq,
		MimeType:     item.MimeType,
		Hash:         item.Hash,
		Source:       item.Source,
		CreatedAt:    item.CreatedAt,
		ExpiresAt:    item.ExpiresAt,
		SizeBytes:    len(item.RawBytes()),
		DownloadPath: fmt.Sprintf("/api/clip/blob?seq=%d", item.Seq),
	}
	if item.IsText() {
		summary.Content = item.Content
	}
	return summary
}
