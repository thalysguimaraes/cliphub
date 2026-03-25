package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/thalysguimaraes/cliphub/internal/agent"
)

type stubAgentRunner struct {
	runErr error
}

func (s stubAgentRunner) Run(context.Context) error {
	return s.runErr
}

func TestRunMainClipboardInitFailure(t *testing.T) {
	origNewAgent := newAgent
	newAgent = func(cfg agent.Config) (agentRunner, error) {
		return nil, &agent.ClipboardInitError{Err: errors.New("clipboard unavailable")}
	}
	t.Cleanup(func() {
		newAgent = origNewAgent
	})

	var logs bytes.Buffer
	origLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() {
		slog.SetDefault(origLogger)
	})

	exitCode := runMain([]string{"-hub", "http://127.0.0.1:1", "-node", "test-node"})
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}

	logOutput := logs.String()
	if !strings.Contains(logOutput, "clipboard init failed") {
		t.Fatalf("expected clipboard init log, got %q", logOutput)
	}
	if !strings.Contains(logOutput, "clipboard unavailable") {
		t.Fatalf("expected underlying error in log, got %q", logOutput)
	}
}
