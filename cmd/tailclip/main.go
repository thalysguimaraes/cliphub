package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/thalysguimaraes/cliphub/internal/discover"
	"github.com/thalysguimaraes/cliphub/internal/hubclient"
)

var hub *hubclient.Client

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Allow --hub flag anywhere.
	hubURL := os.Getenv("CLIPHUB_HUB")
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

	if hubURL == "" {
		resolver := discover.NewResolver(discover.DefaultConfig())
		if url, err := resolver.HubURL(ctx); err == nil {
			hubURL = url
		} else {
			hubURL = "http://localhost:8080"
		}
	}

	var err error
	hub, err = hubclient.New(hubclient.Config{BaseURL: hubURL})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	switch args[0] {
	case "get":
		err = cmdGet(ctx, args[1:])
	case "put":
		err = cmdPut(ctx, args[1:])
	case "history":
		err = cmdHistory(ctx, args[1:])
	case "status":
		err = cmdStatus(ctx)
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

Environment:
  CLIPHUB_HUB        Explicit hub URL override
  CLIPHUB_HOSTNAME   Tailnet hostname used for auto-discovery (default: cliphub)
`)
}

func cmdGet(ctx context.Context, args []string) error {
	var outFile string
	for i, arg := range args {
		if (arg == "-o" || arg == "--output") && i+1 < len(args) {
			outFile = args[i+1]
			break
		}
	}

	item, err := hub.Current(ctx)
	if errors.Is(err, hubclient.ErrNoCurrentClip) {
		fmt.Fprintln(os.Stderr, "(clipboard empty)")
		return nil
	}
	if err != nil {
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

func cmdPut(ctx context.Context, args []string) error {
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

	var payload hubclient.PutRequest

	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}
		if mimeType == "" {
			mimeType = mimeFromExt(filepath.Ext(filePath))
		}
		if strings.HasPrefix(mimeType, "text/") {
			payload = hubclient.PutRequest{Content: string(data), MimeType: mimeType}
		} else {
			payload = hubclient.PutRequest{Data: data, MimeType: mimeType}
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
		payload = hubclient.PutRequest{Content: content, MimeType: mimeType}
	}

	item, err := hub.Put(ctx, payload)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "stored (seq=%d, %s, %d bytes)\n", item.Seq, item.MimeType, len(item.RawBytes()))
	return nil
}

func cmdHistory(ctx context.Context, args []string) error {
	limit := 20
	for i, arg := range args {
		if (arg == "-n" || arg == "--limit") && i+1 < len(args) {
			if n, err := strconv.Atoi(args[i+1]); err == nil && n > 0 {
				limit = n
			}
			break
		}
	}

	items, err := hub.History(ctx, limit)
	if err != nil {
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

func cmdStatus(ctx context.Context) error {
	status, err := hub.Status(ctx)
	if err != nil {
		return err
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
