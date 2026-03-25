package clipboard

import "strings"

// Content represents clipboard data with its MIME type.
type Content struct {
	MimeType string
	Data     []byte
}

// IsText returns true if the content is a text type.
func (c Content) IsText() bool {
	return strings.HasPrefix(c.MimeType, "text/")
}

// Text returns the content as a string. Only meaningful for text types.
func (c Content) Text() string {
	return string(c.Data)
}

// Empty returns true if there is no data.
func (c Content) Empty() bool {
	return len(c.Data) == 0
}

// Clipboard reads and writes the system clipboard with MIME type support.
type Clipboard interface {
	// ReadBest returns the richest available clipboard content.
	// Priority: image/png > text/html > text/plain.
	ReadBest() (Content, error)

	// Write sets the clipboard to the given content.
	Write(Content) error

	// Clear removes clipboard contents from the local system clipboard.
	Clear() error
}

// typePriority defines the preference order for reading clipboard content.
// Higher index = higher priority.
var typePriority = []string{
	"text/plain",
	"text/html",
	"image/png",
}

// bestType picks the highest-priority MIME type from a list of available types.
func bestType(available []string) string {
	set := make(map[string]bool, len(available))
	for _, t := range available {
		// Normalize: "text/plain;charset=utf-8" → "text/plain"
		if idx := strings.IndexByte(t, ';'); idx != -1 {
			t = t[:idx]
		}
		set[strings.TrimSpace(t)] = true
	}

	best := ""
	for _, t := range typePriority {
		if set[t] {
			best = t
		}
	}
	if best == "" && len(available) > 0 {
		// Fall back to text/plain if we have any text type.
		for _, t := range available {
			if strings.HasPrefix(t, "text/") {
				return "text/plain"
			}
		}
	}
	return best
}
