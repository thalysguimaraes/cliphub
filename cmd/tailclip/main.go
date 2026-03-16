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

	"github.com/thalys/cliphub/internal/discover"
	"github.com/thalys/cliphub/internal/protocol"
)

var hubURL string

func main() {
	hubURL = os.Getenv("CLIPHUB_HUB")
	if hubURL == "" {
		if url, err := discover.HubURL(); err == nil {
			hubURL = url
		} else {
			hubURL = "http://localhost:8080"
		}
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
		err = cmdGet(args[1:])
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
  get [-o file]              Print current clip (or save binary to file)
  put [text]                 Send text to clipboard (reads stdin if no args)
  put --file path            Send a file (MIME auto-detected from extension)
  put --mime type [text]     Send with explicit MIME type
  history [-n N]             Show clipboard history
  status                     Show hub status
  pause                      Pause clipboard sync
  resume                     Resume clipboard sync

Flags:
  --hub URL        Hub URL (default: auto-discovered, $CLIPHUB_HUB, or localhost)
`)
}

func cmdGet(args []string) error {
	var outFile string
	for i, arg := range args {
		if (arg == "-o" || arg == "--output") && i+1 < len(args) {
			outFile = args[i+1]
			break
		}
	}

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

	if item.IsText() {
		if outFile != "" {
			return os.WriteFile(outFile, []byte(item.Content), 0o644)
		}
		fmt.Print(item.Content)
		return nil
	}

	// Binary content.
	if outFile != "" {
		if err := os.WriteFile(outFile, item.Data, 0o644); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "saved %s (%d bytes) to %s\n", item.MimeType, len(item.Data), outFile)
		return nil
	}

	fmt.Fprintf(os.Stderr, "[%s, %d bytes] use -o <file> to save\n", item.MimeType, len(item.Data))
	return nil
}

func cmdPut(args []string) error {
	var (
		mimeType string
		filePath string
		textArgs []string
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--mime":
			if i+1 < len(args) {
				mimeType = args[i+1]
				i++
			}
		case "--file":
			if i+1 < len(args) {
				filePath = args[i+1]
				i++
			}
		default:
			textArgs = append(textArgs, args[i])
		}
	}

	var payload map[string]any

	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}
		if mimeType == "" {
			mimeType = mimeFromExt(filepath.Ext(filePath))
		}
		if strings.HasPrefix(mimeType, "text/") {
			payload = map[string]any{"content": string(data), "mime_type": mimeType}
		} else {
			payload = map[string]any{"data": data, "mime_type": mimeType}
		}
	} else {
		var content string
		if len(textArgs) > 0 {
			content = strings.Join(textArgs, " ")
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
		if mimeType == "" {
			mimeType = "text/plain"
		}
		payload = map[string]any{"content": content, "mime_type": mimeType}
	}

	body, _ := json.Marshal(payload)
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
	fmt.Fprintf(os.Stderr, "stored (seq=%d, %s, %d bytes)\n", item.Seq, item.MimeType, len(item.RawBytes()))
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
		age := time.Since(item.CreatedAt).Truncate(time.Second)
		if item.IsText() {
			preview := item.Content
			if len(preview) > 80 {
				preview = preview[:77] + "..."
			}
			preview = strings.ReplaceAll(preview, "\n", "\\n")
			fmt.Printf("#%-4d [%s ago] %s  %s  %q\n", item.Seq, age, item.Source, item.MimeType, preview)
		} else {
			fmt.Printf("#%-4d [%s ago] %s  %s  [%d bytes]\n", item.Seq, age, item.Source, item.MimeType, len(item.Data))
		}
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

func mimeFromExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".png":
		return "image/png"
	case ".html", ".htm":
		return "text/html"
	case ".txt":
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}
