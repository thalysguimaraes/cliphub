package agent

import (
	"sync"

	"github.com/thalysguimaraes/cliphub/internal/clipboard"
	"github.com/thalysguimaraes/cliphub/internal/protocol"
)

// ClipboardMonitor tracks clipboard state and prevents feedback loops.
type ClipboardMonitor struct {
	mu              sync.Mutex
	lastWrittenHash string // Hash of the last item we wrote (from remote).
	lastWrittenMime string // MIME type of the last item we wrote.
	lastSeenHash    string // Hash of the last item we read from clipboard.
	lastSeenMime    string // MIME type of the last item we read.
	pendingHash     string // Hash of content returned by Poll but not yet sent.
	pendingMime     string // MIME type of content returned by Poll but not yet sent.
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

	if hash == m.lastWrittenHash && ct.MimeType == m.lastWrittenMime {
		// Our own write echoing back. Commit as seen.
		m.lastSeenHash = hash
		m.lastSeenMime = ct.MimeType
		return PollOwnWrite, clipboard.Content{}
	}

	// Genuine new content. Don't commit as lastSeen yet — the caller
	// must call MarkSent() after successful transmission so that a
	// failed send doesn't silently drop the item.
	m.pendingHash = hash
	m.pendingMime = ct.MimeType
	return PollNewContent, ct
}

// MarkSent commits the pending poll state after a successful send to the hub.
// If not called, the next Poll() will return the same content again, ensuring
// no local clipboard changes are silently dropped on transient send failures.
func (m *ClipboardMonitor) MarkSent() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastSeenHash = m.pendingHash
	m.lastSeenMime = m.pendingMime
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
