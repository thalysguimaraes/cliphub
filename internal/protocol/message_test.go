package protocol

import (
	"encoding/json"
	"testing"
)

func TestHashContent(t *testing.T) {
	h1 := HashContent("hello")
	h2 := HashContent("hello")
	h3 := HashContent("world")

	if h1 != h2 {
		t.Fatal("same input must produce same hash")
	}
	if h1 == h3 {
		t.Fatal("different input must produce different hash")
	}
	if len(h1) != 64 {
		t.Fatalf("expected 64 hex chars, got %d", len(h1))
	}
}

func TestClipItemJSON(t *testing.T) {
	item := ClipItem{
		Seq:     1,
		Content: "test",
		Hash:    HashContent("test"),
		Source:  "node1",
	}
	data, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}

	var decoded ClipItem
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Seq != item.Seq || decoded.Content != item.Content || decoded.Hash != item.Hash {
		t.Fatal("round-trip mismatch")
	}
}

func TestWSMessageJSON(t *testing.T) {
	msg := WSMessage{
		Type: "clip_update",
		Item: &ClipItem{Seq: 5, Content: "hi"},
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}

	var decoded WSMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Type != "clip_update" || decoded.Item.Seq != 5 {
		t.Fatal("round-trip mismatch")
	}
}
