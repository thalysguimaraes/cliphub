package privacy

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/thalysguimaraes/cliphub/internal/clipboard"
)

type SensitiveClass string

const (
	SensitiveSecret          SensitiveClass = "secret"
	SensitiveOTP             SensitiveClass = "otp"
	SensitivePasswordManager SensitiveClass = "password-manager"
)

var sensitiveClassNames = map[string]SensitiveClass{
	string(SensitiveSecret):          SensitiveSecret,
	string(SensitiveOTP):             SensitiveOTP,
	string(SensitivePasswordManager): SensitivePasswordManager,
}

var passwordManagerFingerprints = []string{
	"1password",
	"agilebits",
	"bitwarden",
	"dashlane",
	"enpass",
	"keepass",
	"keepassxc",
	"lastpass",
	"proton pass",
	"protonpass",
}

var otpPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^\s*\d{6,8}\s*$`),
	regexp.MustCompile(`^\s*\d{3}[-\s]\d{3}\s*$`),
	regexp.MustCompile(`(?i)\b(?:otp|2fa|mfa|totp|verification(?: code)?|security code|auth(?:entication)? code|one[- ]time(?: passcode| password| code)?)\b.*\b\d{4,8}\b`),
	regexp.MustCompile(`(?i)\b\d{4,8}\b.*\b(?:otp|2fa|mfa|totp|verification(?: code)?|security code|auth(?:entication)? code|one[- ]time(?: passcode| password| code)?)\b`),
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`github_pat_[A-Za-z0-9_]{20,}`),
	regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{20,}`),
	regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,}`),
	regexp.MustCompile(`sk_(?:live|test)_[A-Za-z0-9]{16,}`),
	regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._-]{16,}`),
	regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`),
	regexp.MustCompile(`-----BEGIN [A-Z0-9 ]+PRIVATE KEY-----`),
}

type Config struct {
	IgnoreApps       []string
	IgnoreProcesses  []string
	SensitiveClasses map[SensitiveClass]struct{}
	ClearOnBlock     bool
}

type Context struct {
	AppName     string
	BundleID    string
	ProcessName string
}

type Decision struct {
	Block          bool
	ClearClipboard bool
	Rule           string
	Matched        string
}

func ParseCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = normalize(part)
		if part == "" {
			continue
		}
		values = append(values, part)
	}
	return values
}

func ParseSensitiveClasses(raw string) (map[SensitiveClass]struct{}, error) {
	names := ParseCSV(raw)
	if len(names) == 0 {
		return nil, nil
	}

	classes := make(map[SensitiveClass]struct{}, len(names))
	for _, name := range names {
		class, ok := sensitiveClassNames[name]
		if !ok {
			return nil, fmt.Errorf("unknown sensitive class %q", name)
		}
		classes[class] = struct{}{}
	}
	return classes, nil
}

func NewConfig(ignoreApps, ignoreProcesses []string, classes map[SensitiveClass]struct{}, clearOnBlock bool) Config {
	cfg := Config{
		IgnoreApps:      normalizeSlice(ignoreApps),
		IgnoreProcesses: normalizeSlice(ignoreProcesses),
		ClearOnBlock:    clearOnBlock,
	}
	if len(classes) > 0 {
		cfg.SensitiveClasses = make(map[SensitiveClass]struct{}, len(classes))
		for class := range classes {
			cfg.SensitiveClasses[class] = struct{}{}
		}
	}
	return cfg
}

func (c Config) Empty() bool {
	return len(c.IgnoreApps) == 0 && len(c.IgnoreProcesses) == 0 && len(c.SensitiveClasses) == 0
}

func (c Config) UsesContext() bool {
	return len(c.IgnoreApps) > 0 || len(c.IgnoreProcesses) > 0 || c.HasSensitiveClass(SensitivePasswordManager)
}

func (c Config) HasSensitiveClass(class SensitiveClass) bool {
	if len(c.SensitiveClasses) == 0 {
		return false
	}
	_, ok := c.SensitiveClasses[class]
	return ok
}

func (c Config) Decide(ctx Context, ct clipboard.Content) Decision {
	if c.Empty() || ct.Empty() {
		return Decision{}
	}

	if matched := matchConfigured(c.IgnoreApps, ctx.AppName, ctx.BundleID); matched != "" {
		return c.blocked("ignore-app", matched)
	}
	if matched := matchConfigured(c.IgnoreProcesses, ctx.ProcessName); matched != "" {
		return c.blocked("ignore-process", matched)
	}
	if c.HasSensitiveClass(SensitivePasswordManager) {
		if matched := detectPasswordManager(ctx); matched != "" {
			return c.blocked(string(SensitivePasswordManager), matched)
		}
	}

	if !ct.IsText() {
		return Decision{}
	}

	text := strings.TrimSpace(ct.Text())
	if text == "" {
		return Decision{}
	}

	if c.HasSensitiveClass(SensitiveOTP) && looksLikeOTP(text) {
		return c.blocked(string(SensitiveOTP), "otp-like text")
	}
	if c.HasSensitiveClass(SensitiveSecret) {
		if matched := looksLikeSecret(text); matched != "" {
			return c.blocked(string(SensitiveSecret), matched)
		}
	}

	return Decision{}
}

func (c Config) blocked(rule string, matched string) Decision {
	return Decision{
		Block:          true,
		ClearClipboard: c.ClearOnBlock,
		Rule:           rule,
		Matched:        matched,
	}
}

func detectPasswordManager(ctx Context) string {
	return matchConfigured(passwordManagerFingerprints, ctx.AppName, ctx.BundleID, ctx.ProcessName)
}

func looksLikeOTP(text string) bool {
	for _, pattern := range otpPatterns {
		if pattern.MatchString(text) {
			return true
		}
	}
	return false
}

func looksLikeSecret(text string) string {
	for _, pattern := range secretPatterns {
		if match := pattern.FindString(text); match != "" {
			return match
		}
	}
	return ""
}

func matchConfigured(rules []string, candidates ...string) string {
	if len(rules) == 0 {
		return ""
	}

	normalizedCandidates := normalizeSlice(candidates)
	for _, rule := range rules {
		for _, candidate := range normalizedCandidates {
			if candidate == "" {
				continue
			}
			if candidate == rule || strings.Contains(candidate, rule) {
				return candidate
			}
		}
	}
	return ""
}

func normalizeSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = normalize(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	return normalized
}

func normalize(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
