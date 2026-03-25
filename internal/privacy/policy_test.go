package privacy

import (
	"testing"

	"github.com/thalysguimaraes/cliphub/internal/clipboard"
)

func TestParseSensitiveClassesRejectsUnknown(t *testing.T) {
	if _, err := ParseSensitiveClasses("secret,unknown"); err == nil {
		t.Fatal("expected unknown class error")
	}
}

func TestDecisionIgnoresConfiguredApp(t *testing.T) {
	cfg := NewConfig([]string{"1password"}, nil, nil, false)

	decision := cfg.Decide(Context{AppName: "1Password 8"}, textContent("copy me"))
	if !decision.Block || decision.Rule != "ignore-app" {
		t.Fatalf("expected ignore-app decision, got %+v", decision)
	}
}

func TestDecisionIgnoresConfiguredProcess(t *testing.T) {
	cfg := NewConfig(nil, []string{"keepassxc"}, nil, false)

	decision := cfg.Decide(Context{ProcessName: "KeePassXC"}, textContent("copy me"))
	if !decision.Block || decision.Rule != "ignore-process" {
		t.Fatalf("expected ignore-process decision, got %+v", decision)
	}
}

func TestDecisionBlocksPasswordManagerContext(t *testing.T) {
	cfg := NewConfig(nil, nil, mustClasses(t, "password-manager"), false)

	decision := cfg.Decide(Context{BundleID: "com.1password.1password"}, textContent("copy me"))
	if !decision.Block || decision.Rule != "password-manager" {
		t.Fatalf("expected password-manager decision, got %+v", decision)
	}
}

func TestDecisionBlocksOTP(t *testing.T) {
	cfg := NewConfig(nil, nil, mustClasses(t, "otp"), true)

	decision := cfg.Decide(Context{}, textContent("Your verification code is 123456"))
	if !decision.Block || decision.Rule != "otp" || !decision.ClearClipboard {
		t.Fatalf("expected otp decision with clear, got %+v", decision)
	}
}

func TestDecisionBlocksSecrets(t *testing.T) {
	cfg := NewConfig(nil, nil, mustClasses(t, "secret"), false)

	decision := cfg.Decide(Context{}, textContent("Bearer super-secret-token-value"))
	if !decision.Block || decision.Rule != "secret" {
		t.Fatalf("expected secret decision, got %+v", decision)
	}
}

func TestDecisionIsOptIn(t *testing.T) {
	cfg := NewConfig(nil, nil, nil, false)

	decision := cfg.Decide(Context{AppName: "1Password"}, textContent("123456"))
	if decision.Block {
		t.Fatalf("expected opt-in behavior, got %+v", decision)
	}
}

func textContent(value string) clipboard.Content {
	return clipboard.Content{MimeType: "text/plain", Data: []byte(value)}
}

func mustClasses(t *testing.T, raw string) map[SensitiveClass]struct{} {
	t.Helper()

	classes, err := ParseSensitiveClasses(raw)
	if err != nil {
		t.Fatalf("ParseSensitiveClasses(%q): %v", raw, err)
	}
	return classes
}
