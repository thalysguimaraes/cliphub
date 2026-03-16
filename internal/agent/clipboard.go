package agent

import (
	"sync"

	"github.com/atotto/clipboard"
	"github.com/thalys/cliphub/internal/protocol"
)

// Clipboard abstracts clipboard access for testability.
type Clipboard interface {
	ReadAll() (string, error)
	WriteAll(string) error
}

// SystemClipboard wraps the OS clipboard.
type SystemClipboard struct{}

func (SystemClipboard) ReadAll() (string, error) { return clipboard.ReadAll() }
func (SystemClipboard) WriteAll(s string) error   { return clipboard.WriteAll(s) }

// ClipboardMonitor tracks clipboard state and prevents feedback loops.
type ClipboardMonitor struct {
	mu              sync.Mutex
	lastWrittenHash string // Hash of the last item we wrote (from remote).
	lastSeenHash    string // Hash of the last item we read from clipboard.
	clip            Clipboard
}

// NewClipboardMonitor creates a monitor with the given clipboard backend.
func NewClipboardMonitor(clip Clipboard) *ClipboardMonitor {
	return &ClipboardMonitor{clip: clip}
}

// PollResult represents what happened on a clipboard poll.
type PollResult int

const (
	PollNoChange      PollResult = iota // Clipboard unchanged.
	PollOwnWrite                        // We wrote this ourselves (remote update echo).
	PollNewContent                      // Genuine new local content.
	PollError                           // Error reading clipboard.
)

// Poll reads the clipboard and returns what happened plus the content if new.
func (m *ClipboardMonitor) Poll() (PollResult, string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	content, err := m.clip.ReadAll()
	if err != nil {
		return PollError, ""
	}
	if content == "" {
		return PollNoChange, ""
	}

	hash := protocol.HashContent(content)

	if hash == m.lastSeenHash {
		return PollNoChange, ""
	}

	m.lastSeenHash = hash

	if hash == m.lastWrittenHash {
		return PollOwnWrite, ""
	}

	return PollNewContent, content
}

// ApplyRemote writes a remote clipboard item to the local clipboard.
// Returns an error if the write fails.
func (m *ClipboardMonitor) ApplyRemote(content string) error {
	hash := protocol.HashContent(content)

	m.mu.Lock()
	m.lastWrittenHash = hash
	m.mu.Unlock()

	if err := m.clip.WriteAll(content); err != nil {
		return err
	}

	m.mu.Lock()
	m.lastSeenHash = hash
	m.mu.Unlock()

	return nil
}
