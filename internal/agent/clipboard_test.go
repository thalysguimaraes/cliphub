package agent

import (
	"sync"
	"testing"

	"github.com/thalysguimaraes/cliphub/internal/clipboard"
)

// fakeClipboard is an in-memory clipboard for testing.
type fakeClipboard struct {
	mu      sync.RWMutex
	content clipboard.Content
}

func (f *fakeClipboard) ReadBest() (clipboard.Content, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	return cloneContent(f.content), nil
}

func (f *fakeClipboard) Write(c clipboard.Content) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.content = cloneContent(c)
	return nil
}

func (f *fakeClipboard) Clear() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.content = clipboard.Content{}
	return nil
}

func (f *fakeClipboard) SetContent(c clipboard.Content) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.content = cloneContent(c)
}

func (f *fakeClipboard) Content() clipboard.Content {
	f.mu.RLock()
	defer f.mu.RUnlock()

	return cloneContent(f.content)
}

func cloneContent(c clipboard.Content) clipboard.Content {
	cloned := c
	if c.Data != nil {
		cloned.Data = append([]byte(nil), c.Data...)
	}
	return cloned
}

func textContent(s string) clipboard.Content {
	return clipboard.Content{MimeType: "text/plain", Data: []byte(s)}
}

func TestPollNoChange(t *testing.T) {
	clip := &fakeClipboard{content: textContent("hello")}
	m := NewClipboardMonitor(clip)

	result, ct := m.Poll()
	if result != PollNewContent || ct.Text() != "hello" {
		t.Fatalf("expected new content, got %v %q", result, ct.Text())
	}
	m.MarkSent() // Commit after successful "send".

	result, _ = m.Poll()
	if result != PollNoChange {
		t.Fatalf("expected no change, got %v", result)
	}
}

func TestPollDetectsLocalChange(t *testing.T) {
	clip := &fakeClipboard{content: textContent("first")}
	m := NewClipboardMonitor(clip)
	m.Poll()
	m.MarkSent()

	clip.SetContent(textContent("second"))
	result, ct := m.Poll()
	if result != PollNewContent || ct.Text() != "second" {
		t.Fatalf("expected new content 'second', got %v %q", result, ct.Text())
	}
}

func TestPollRetriesWithoutMarkSent(t *testing.T) {
	clip := &fakeClipboard{content: textContent("hello")}
	m := NewClipboardMonitor(clip)

	// First poll sees new content.
	result, ct := m.Poll()
	if result != PollNewContent || ct.Text() != "hello" {
		t.Fatalf("expected new content, got %v", result)
	}

	// Simulate send failure: don't call MarkSent().
	// Next poll should return the same content again.
	result, ct = m.Poll()
	if result != PollNewContent || ct.Text() != "hello" {
		t.Fatalf("expected retry of unsent content, got %v %q", result, ct.Text())
	}

	// Now simulate success.
	m.MarkSent()

	// Next poll should see no change.
	result, _ = m.Poll()
	if result != PollNoChange {
		t.Fatalf("expected no change after MarkSent, got %v", result)
	}
}

func TestPollNewContentOverwritesPending(t *testing.T) {
	clip := &fakeClipboard{content: textContent("first")}
	m := NewClipboardMonitor(clip)

	// Poll sees "first", don't mark sent (simulating failed send).
	m.Poll()

	// User copies something new before retry succeeds.
	clip.SetContent(textContent("second"))
	result, ct := m.Poll()
	if result != PollNewContent || ct.Text() != "second" {
		t.Fatalf("expected new content 'second', got %v %q", result, ct.Text())
	}
}

func TestApplyRemoteDoesNotEcho(t *testing.T) {
	clip := &fakeClipboard{content: textContent("local")}
	m := NewClipboardMonitor(clip)
	m.Poll()
	m.MarkSent()

	if err := m.ApplyRemote(textContent("remote")); err != nil {
		t.Fatal(err)
	}

	result, _ := m.Poll()
	if result != PollNoChange {
		t.Fatalf("expected no change after applying remote, got %v", result)
	}
}

func TestApplyRemoteThenLocalChange(t *testing.T) {
	clip := &fakeClipboard{content: textContent("local")}
	m := NewClipboardMonitor(clip)
	m.Poll()
	m.MarkSent()

	m.ApplyRemote(textContent("remote"))

	clip.SetContent(textContent("user-copied"))
	result, ct := m.Poll()
	if result != PollNewContent || ct.Text() != "user-copied" {
		t.Fatalf("expected new local content, got %v %q", result, ct.Text())
	}
}

func TestPollEmptyClipboard(t *testing.T) {
	clip := &fakeClipboard{content: clipboard.Content{}}
	m := NewClipboardMonitor(clip)

	result, _ := m.Poll()
	if result != PollNoChange {
		t.Fatalf("empty clipboard should be no-change, got %v", result)
	}
}

func TestMultipleRemoteApplies(t *testing.T) {
	clip := &fakeClipboard{content: clipboard.Content{}}
	m := NewClipboardMonitor(clip)

	m.ApplyRemote(textContent("first-remote"))
	result, _ := m.Poll()
	if result != PollNoChange {
		t.Fatalf("expected no change after remote apply, got %v", result)
	}

	m.ApplyRemote(textContent("second-remote"))
	result, _ = m.Poll()
	if result != PollNoChange {
		t.Fatalf("expected no change after second remote apply, got %v", result)
	}

	clip.SetContent(textContent("local-change"))
	result, ct := m.Poll()
	if result != PollNewContent || ct.Text() != "local-change" {
		t.Fatalf("expected new local content, got %v %q", result, ct.Text())
	}
}

func TestBinaryContent(t *testing.T) {
	pngData := clipboard.Content{MimeType: "image/png", Data: []byte{0x89, 0x50, 0x4e, 0x47}}
	clip := &fakeClipboard{content: pngData}
	m := NewClipboardMonitor(clip)

	result, ct := m.Poll()
	if result != PollNewContent || ct.MimeType != "image/png" {
		t.Fatalf("expected new image content, got %v %s", result, ct.MimeType)
	}
	m.MarkSent()

	// Apply remote image, should not echo.
	otherPng := clipboard.Content{MimeType: "image/png", Data: []byte{0x89, 0x50, 0x4e, 0x48}}
	m.ApplyRemote(otherPng)
	result, _ = m.Poll()
	if result != PollNoChange {
		t.Fatalf("expected no change after remote image apply, got %v", result)
	}
}
