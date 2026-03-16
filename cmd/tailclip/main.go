package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/thalys/cliphub/internal/protocol"
)

var hubURL string

func main() {
	hubURL = os.Getenv("CLIPHUB_HUB")
	if hubURL == "" {
		hubURL = "http://localhost:8080"
	}

	// Allow --hub flag anywhere.
	args := os.Args[1:]
	for i, arg := range args {
		if arg == "--hub" && i+1 < len(args) {
			hubURL = args[i+1]
			args = append(args[:i], args[i+2:]...)
			break
		}
		if strings.HasPrefix(arg, "--hub=") {
			hubURL = strings.TrimPrefix(arg, "--hub=")
			args = append(args[:i], args[i+1:]...)
			break
		}
	}

	if len(args) == 0 {
		usage()
		os.Exit(1)
	}

	var err error
	switch args[0] {
	case "get":
		err = cmdGet()
	case "put":
		err = cmdPut(args[1:])
	case "history":
		err = cmdHistory(args[1:])
	case "status":
		err = cmdStatus()
	case "pause":
		err = cmdPause()
	case "resume":
		err = cmdResume()
	default:
		usage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: tailclip <command> [args]

Commands:
  get              Print current clipboard content
  put [text]       Send text to clipboard (reads stdin if no args)
  history [-n N]   Show clipboard history
  status           Show hub status
  pause            Pause clipboard sync
  resume           Resume clipboard sync

Flags:
  --hub URL        Hub URL (default: $CLIPHUB_HUB or http://localhost:8080)
`)
}

func cmdGet() error {
	resp, err := http.Get(hubURL + "/api/clip")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		fmt.Fprintln(os.Stderr, "(clipboard empty)")
		return nil
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("hub returned %d", resp.StatusCode)
	}

	var item protocol.ClipItem
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return err
	}
	fmt.Print(item.Content)
	return nil
}

func cmdPut(args []string) error {
	var content string
	if len(args) > 0 {
		content = strings.Join(args, " ")
	} else {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		content = string(data)
	}

	if content == "" {
		return fmt.Errorf("no content provided")
	}

	body, _ := json.Marshal(map[string]string{"content": content})
	resp, err := http.Post(hubURL+"/api/clip", "application/json", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("hub: %s (%d)", strings.TrimSpace(string(msg)), resp.StatusCode)
	}

	var item protocol.ClipItem
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	fmt.Fprintf(os.Stderr, "stored (seq=%d, %d bytes)\n", item.Seq, len(content))
	return nil
}

func cmdHistory(args []string) error {
	limit := "20"
	for i, arg := range args {
		if (arg == "-n" || arg == "--limit") && i+1 < len(args) {
			limit = args[i+1]
			break
		}
	}

	resp, err := http.Get(hubURL + "/api/clip/history?limit=" + limit)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("hub returned %d", resp.StatusCode)
	}

	var items []protocol.ClipItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return err
	}

	for _, item := range items {
		preview := item.Content
		if len(preview) > 80 {
			preview = preview[:77] + "..."
		}
		preview = strings.ReplaceAll(preview, "\n", "\\n")
		age := time.Since(item.CreatedAt).Truncate(time.Second)
		fmt.Printf("#%-4d [%s ago] %s  %q\n", item.Seq, age, item.Source, preview)
	}
	return nil
}

func cmdStatus() error {
	resp, err := http.Get(hubURL + "/api/status")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("hub returned %d", resp.StatusCode)
	}

	var status map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	for k, v := range status {
		fmt.Printf("%-12s %v\n", k+":", v)
	}
	return nil
}

func cmdPause() error {
	path := pauseFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte("paused\n"), 0o644); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "clipboard sync paused")
	return nil
}

func cmdResume() error {
	path := pauseFilePath()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	fmt.Fprintln(os.Stderr, "clipboard sync resumed")
	return nil
}

func pauseFilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "cliphub", "paused")
}
