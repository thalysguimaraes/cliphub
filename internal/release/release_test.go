package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractChangelogSectionPrefersVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "CHANGELOG.md")
	content := strings.Join([]string{
		"# Changelog",
		"",
		"## Unreleased",
		"",
		"- pending change",
		"",
		"## v1.2.3",
		"",
		"- shipped change",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	body, section, err := extractChangelogSection(path, "v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if section != "v1.2.3" {
		t.Fatalf("expected version section, got %q", section)
	}
	if body != "- shipped change" {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestExtractChangelogSectionFallsBackToUnreleased(t *testing.T) {
	path := filepath.Join(t.TempDir(), "CHANGELOG.md")
	content := strings.Join([]string{
		"# Changelog",
		"",
		"## Unreleased",
		"",
		"### Added",
		"",
		"- automated release notes",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	body, section, err := extractChangelogSection(path, "v9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	if section != "Unreleased" {
		t.Fatalf("expected Unreleased section, got %q", section)
	}
	if !strings.Contains(body, "automated release notes") {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestRenderReleaseNotesIncludesAssetsSummary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "CHANGELOG.md")
	content := strings.Join([]string{
		"# Changelog",
		"",
		"## Unreleased",
		"",
		"### Added",
		"",
		"- deterministic archives",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	notes, err := renderReleaseNotes(path, "v1.0.0", []Artifact{
		{
			Name: "clipd_v1.0.0_linux_amd64.tar.gz",
			Target: Target{
				Binary: "clipd",
				GOOS:   "linux",
				GOARCH: "amd64",
			},
		},
		{
			Name: "cliphub_v1.0.0_linux_amd64.tar.gz",
			Target: Target{
				Binary: "cliphub",
				GOOS:   "linux",
				GOARCH: "amd64",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"# ClipHub v1.0.0",
		"- deterministic archives",
		"- `cliphub`: `linux/amd64`",
		"cliphub_v1.0.0_checksums.txt",
		"cliphub_v1.0.0_artifacts.json",
	} {
		if !strings.Contains(notes, want) {
			t.Fatalf("expected %q in notes:\n%s", want, notes)
		}
	}
}
