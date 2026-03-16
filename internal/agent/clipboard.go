package agent

import (
	"sync"

	"github.com/thalys/cliphub/internal/clipboard"
	"github.com/thalys/cliphub/internal/protocol"
)

// ClipboardMonitor tracks clipboard state and prevents feedback loops.
type ClipboardMonitor struct {
	mu              sync.Mutex
	lastWrittenHash string // Hash of the last item we wrote (from remote).
	lastWrittenMime string // MIME type of the last item we wrote.
	lastSeenHash    string // Hash of the last item we read from clipboard.
	lastSeenMime    string // MIME type of the last item we read.
	clip            clipboard.Clipboard
}

// NewClipboardMonitor creates a monitor with the given clipboard backend.
func NewClipboardMonitor(clip clipboard.Clipboard) *ClipboardMonitor {
	return &ClipboardMonitor{clip: clip}
}

// PollResult represents what happened on a clipboard poll.
type PollResult int

const (
	PollNoChange  PollResult = iota // Clipboard unchanged.
	PollOwnWrite                    // We wrote this ourselves (remote update echo).
	PollNewContent                  // Genuine new local content.
	PollError                       // Error reading clipboard.
)

// Poll reads the clipboard and returns what happened plus the content if new.
func (m *ClipboardMonitor) Poll() (PollResult, clipboard.Content) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ct, err := m.clip.ReadBest()
	if err != nil {
		return PollError, clipboard.Content{}
	}
	if ct.Empty() {
		return PollNoChange, clipboard.Content{}
	}

	hash := protocol.HashBytes(ct.Data)

	if hash == m.lastSeenHash && ct.MimeType == m.lastSeenMime {
		return PollNoChange, clipboard.Content{}
	}

	m.lastSeenHash = hash
	m.lastSeenMime = ct.MimeType

	if hash == m.lastWrittenHash && ct.MimeType == m.lastWrittenMime {
		return PollOwnWrite, clipboard.Content{}
	}

	return PollNewContent, ct
}

// ApplyRemote writes a remote clipboard item to the local clipboard.
// After writing, it reads back the clipboard to capture any format conversion
// the platform may have done, ensuring the next poll won't see a false change.
func (m *ClipboardMonitor) ApplyRemote(ct clipboard.Content) error {
	hash := protocol.HashBytes(ct.Data)

	m.mu.Lock()
	m.lastWrittenHash = hash
	m.lastWrittenMime = ct.MimeType
	m.mu.Unlock()

	if err := m.clip.Write(ct); err != nil {
		return err
	}

	// Read back what the platform actually stored. If it converted the format
	// (e.g., we wrote HTML but it reads back as plain text), update our hashes
	// to match the read-back so the next poll doesn't see a false new content.
	readBack, err := m.clip.ReadBest()
	if err == nil && !readBack.Empty() {
		readBackHash := protocol.HashBytes(readBack.Data)
		m.mu.Lock()
		m.lastSeenHash = readBackHash
		m.lastSeenMime = readBack.MimeType
		m.lastWrittenHash = readBackHash
		m.lastWrittenMime = readBack.MimeType
		m.mu.Unlock()
	} else {
		m.mu.Lock()
		m.lastSeenHash = hash
		m.lastSeenMime = ct.MimeType
		m.mu.Unlock()
	}

	return nil
}
