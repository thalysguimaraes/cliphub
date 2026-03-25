package release

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Target struct {
	Binary string `json:"binary"`
	GOOS   string `json:"goos"`
	GOARCH string `json:"goarch"`
}

func (t Target) BinaryName() string {
	if t.GOOS == "windows" {
		return t.Binary + ".exe"
	}
	return t.Binary
}

func (t Target) ArchiveName(version string) string {
	base := fmt.Sprintf("%s_%s_%s_%s", t.Binary, sanitizeVersion(version), t.GOOS, t.GOARCH)
	if t.GOOS == "windows" {
		return base + ".zip"
	}
	return base + ".tar.gz"
}

func (t Target) Format() string {
	if t.GOOS == "windows" {
		return "zip"
	}
	return "tar.gz"
}

type Options struct {
	RepoRoot        string
	DistDir         string
	Version         string
	SourceDateEpoch int64
}

type VerifyOptions struct {
	DistDir  string
	Version  string
	RepoRoot string
}

type Artifact struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Target Target `json:"target"`
	Format string `json:"format"`
}

type Manifest struct {
	Version         string     `json:"version"`
	SourceDateEpoch int64      `json:"sourceDateEpoch"`
	ChecksumsFile   string     `json:"checksumsFile"`
	NotesFile       string     `json:"notesFile"`
	Artifacts       []Artifact `json:"artifacts"`
}

type Result struct {
	Version         string
	DistDir         string
	ChecksumsPath   string
	NotesPath       string
	ManifestPath    string
	SourceDateEpoch int64
	Artifacts       []Artifact
}

var DefaultTargets = []Target{
	{Binary: "clipd", GOOS: "darwin", GOARCH: "amd64"},
	{Binary: "clipd", GOOS: "darwin", GOARCH: "arm64"},
	{Binary: "clipd", GOOS: "linux", GOARCH: "amd64"},
	{Binary: "clipd", GOOS: "windows", GOARCH: "amd64"},
	{Binary: "cliphub", GOOS: "linux", GOARCH: "amd64"},
	{Binary: "tailclip", GOOS: "darwin", GOARCH: "amd64"},
	{Binary: "tailclip", GOOS: "darwin", GOARCH: "arm64"},
	{Binary: "tailclip", GOOS: "linux", GOARCH: "amd64"},
	{Binary: "tailclip", GOOS: "windows", GOARCH: "amd64"},
}

func Build(ctx context.Context, opts Options) (Result, error) {
	repoRoot, err := normalizeRepoRoot(opts.RepoRoot)
	if err != nil {
		return Result{}, err
	}
	version := strings.TrimSpace(opts.Version)
	if version == "" {
		return Result{}, errors.New("version is required")
	}

	distDir := opts.DistDir
	if distDir == "" {
		distDir = filepath.Join(repoRoot, "dist", "release")
	}
	if !filepath.IsAbs(distDir) {
		distDir = filepath.Join(repoRoot, distDir)
	}

	sourceDateEpoch := opts.SourceDateEpoch
	if sourceDateEpoch == 0 {
		sourceDateEpoch, err = resolveSourceDateEpoch(ctx, repoRoot, "HEAD")
		if err != nil {
			return Result{}, err
		}
	}
	modTime := time.Unix(sourceDateEpoch, 0).UTC()

	if err := os.RemoveAll(distDir); err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		return Result{}, err
	}

	stagingRoot := filepath.Join(distDir, ".staging")
	if err := os.MkdirAll(stagingRoot, 0o755); err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(stagingRoot)

	docPaths := []string{
		filepath.Join(repoRoot, "README.md"),
		filepath.Join(repoRoot, "LICENSE"),
	}

	var artifacts []Artifact
	for _, target := range DefaultTargets {
		artifact, err := buildTarget(ctx, repoRoot, stagingRoot, distDir, version, modTime, target, docPaths)
		if err != nil {
			return Result{}, err
		}
		artifacts = append(artifacts, artifact)
	}

	sort.Slice(artifacts, func(i, j int) bool {
		return artifacts[i].Name < artifacts[j].Name
	})

	checksumsPath := filepath.Join(distDir, checksumsFileName(version))
	if err := writeChecksums(checksumsPath, artifacts); err != nil {
		return Result{}, err
	}

	notesPath := filepath.Join(distDir, notesFileName(version))
	notes, err := renderReleaseNotes(filepath.Join(repoRoot, "CHANGELOG.md"), version, artifacts)
	if err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(notesPath, []byte(notes), 0o644); err != nil {
		return Result{}, err
	}

	manifestPath := filepath.Join(distDir, manifestFileName(version))
	manifest := Manifest{
		Version:         version,
		SourceDateEpoch: sourceDateEpoch,
		ChecksumsFile:   filepath.Base(checksumsPath),
		NotesFile:       filepath.Base(notesPath),
		Artifacts:       artifacts,
	}
	if err := writeManifest(manifestPath, manifest); err != nil {
		return Result{}, err
	}

	return Result{
		Version:         version,
		DistDir:         distDir,
		ChecksumsPath:   checksumsPath,
		NotesPath:       notesPath,
		ManifestPath:    manifestPath,
		SourceDateEpoch: sourceDateEpoch,
		Artifacts:       artifacts,
	}, nil
}

func Verify(opts VerifyOptions) (Result, error) {
	repoRoot, err := normalizeRepoRoot(opts.RepoRoot)
	if err != nil {
		return Result{}, err
	}
	version := strings.TrimSpace(opts.Version)
	if version == "" {
		return Result{}, errors.New("version is required")
	}

	distDir := opts.DistDir
	if distDir == "" {
		distDir = filepath.Join(repoRoot, "dist", "release")
	}
	if !filepath.IsAbs(distDir) {
		distDir = filepath.Join(repoRoot, distDir)
	}

	checksumsPath := filepath.Join(distDir, checksumsFileName(version))
	notesPath := filepath.Join(distDir, notesFileName(version))
	manifestPath := filepath.Join(distDir, manifestFileName(version))

	entries, err := readChecksums(checksumsPath)
	if err != nil {
		return Result{}, err
	}
	if _, err := os.Stat(notesPath); err != nil {
		return Result{}, fmt.Errorf("release notes missing: %w", err)
	}

	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return Result{}, err
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return Result{}, err
	}

	var artifacts []Artifact
	for _, entry := range entries {
		path := filepath.Join(distDir, entry.Name)
		sum, err := fileSHA256(path)
		if err != nil {
			return Result{}, err
		}
		if sum != entry.Sum {
			return Result{}, fmt.Errorf("checksum mismatch for %s: expected %s, got %s", entry.Name, entry.Sum, sum)
		}
		artifacts = append(artifacts, Artifact{
			Name:   entry.Name,
			Path:   entry.Name,
			SHA256: sum,
		})
	}

	if len(artifacts) != len(manifest.Artifacts) {
		return Result{}, fmt.Errorf("artifact count mismatch: checksums=%d manifest=%d", len(artifacts), len(manifest.Artifacts))
	}
	for i, manifestArtifact := range manifest.Artifacts {
		if artifacts[i].Name != manifestArtifact.Name {
			return Result{}, fmt.Errorf("manifest order mismatch at %d: checksum=%s manifest=%s", i, artifacts[i].Name, manifestArtifact.Name)
		}
		if artifacts[i].SHA256 != manifestArtifact.SHA256 {
			return Result{}, fmt.Errorf("manifest checksum mismatch for %s", manifestArtifact.Name)
		}
		artifacts[i].Format = manifestArtifact.Format
		artifacts[i].Target = manifestArtifact.Target
	}

	return Result{
		Version:         version,
		DistDir:         distDir,
		ChecksumsPath:   checksumsPath,
		NotesPath:       notesPath,
		ManifestPath:    manifestPath,
		SourceDateEpoch: manifest.SourceDateEpoch,
		Artifacts:       artifacts,
	}, nil
}

func ResolveVersion(ctx context.Context, repoRoot string) (string, error) {
	return runOutput(ctx, repoRoot, "git", "describe", "--tags", "--always", "--dirty")
}

func ResolveSourceDateEpoch(ctx context.Context, repoRoot, ref string) (int64, error) {
	return resolveSourceDateEpoch(ctx, repoRoot, ref)
}

func checksumsFileName(version string) string {
	return fmt.Sprintf("cliphub_%s_checksums.txt", sanitizeVersion(version))
}

func notesFileName(version string) string {
	return fmt.Sprintf("cliphub_%s_release_notes.md", sanitizeVersion(version))
}

func manifestFileName(version string) string {
	return fmt.Sprintf("cliphub_%s_artifacts.json", sanitizeVersion(version))
}

func normalizeRepoRoot(repoRoot string) (string, error) {
	if repoRoot == "" {
		repoRoot = "."
	}
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", err
	}
	return root, nil
}

func resolveSourceDateEpoch(ctx context.Context, repoRoot, ref string) (int64, error) {
	value, err := runOutput(ctx, repoRoot, "git", "log", "-1", "--format=%ct", ref)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(value, 10, 64)
}

func buildTarget(ctx context.Context, repoRoot, stagingRoot, distDir, version string, modTime time.Time, target Target, docPaths []string) (Artifact, error) {
	stagingName := strings.TrimSuffix(target.ArchiveName(version), filepath.Ext(target.ArchiveName(version)))
	if target.GOOS != "windows" {
		stagingName = strings.TrimSuffix(stagingName, ".tar")
	}
	stagingDir := filepath.Join(stagingRoot, stagingName)
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return Artifact{}, err
	}

	binaryPath := filepath.Join(stagingDir, target.BinaryName())
	if err := buildBinary(ctx, repoRoot, target, version, binaryPath); err != nil {
		return Artifact{}, err
	}

	for _, docPath := range docPaths {
		dst := filepath.Join(stagingDir, filepath.Base(docPath))
		if err := copyFile(docPath, dst, 0o644); err != nil {
			return Artifact{}, err
		}
	}

	archivePath := filepath.Join(distDir, target.ArchiveName(version))
	if target.GOOS == "windows" {
		if err := writeZip(archivePath, stagingName, stagingDir, modTime); err != nil {
			return Artifact{}, err
		}
	} else {
		if err := writeTarGz(archivePath, stagingName, stagingDir, modTime); err != nil {
			return Artifact{}, err
		}
	}

	sum, err := fileSHA256(archivePath)
	if err != nil {
		return Artifact{}, err
	}

	return Artifact{
		Name:   filepath.Base(archivePath),
		Path:   filepath.Base(archivePath),
		SHA256: sum,
		Target: target,
		Format: target.Format(),
	}, nil
}

func buildBinary(ctx context.Context, repoRoot string, target Target, version, outputPath string) error {
	cmd := exec.CommandContext(
		ctx,
		"go",
		"build",
		"-trimpath",
		"-buildvcs=false",
		"-ldflags",
		fmt.Sprintf("-s -w -X main.version=%s", version),
		"-o",
		outputPath,
		"./cmd/"+target.Binary,
	)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"CGO_ENABLED=0",
		"GOOS="+target.GOOS,
		"GOARCH="+target.GOARCH,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func writeTarGz(dstPath, rootName, stagingDir string, modTime time.Time) error {
	out, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer out.Close()

	gzw, err := gzip.NewWriterLevel(out, gzip.BestCompression)
	if err != nil {
		return err
	}
	gzw.Name = ""
	gzw.Comment = ""
	gzw.ModTime = modTime
	defer gzw.Close()

	tw := tar.NewWriter(gzw)
	defer tw.Close()

	entries, err := archiveEntries(stagingDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := addTarEntry(tw, filepath.Join(stagingDir, entry), filepath.ToSlash(filepath.Join(rootName, entry)), modTime); err != nil {
			return err
		}
	}
	return nil
}

func writeZip(dstPath, rootName, stagingDir string, modTime time.Time) error {
	out, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer out.Close()

	zw := zip.NewWriter(out)
	defer zw.Close()

	entries, err := archiveEntries(stagingDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := addZipEntry(zw, filepath.Join(stagingDir, entry), filepath.ToSlash(filepath.Join(rootName, entry)), modTime); err != nil {
			return err
		}
	}
	return nil
}

func archiveEntries(dir string) ([]string, error) {
	list, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	entries := make([]string, 0, len(list))
	for _, entry := range list {
		entries = append(entries, entry.Name())
	}
	sort.Strings(entries)
	return entries, nil
}

func addTarEntry(tw *tar.Writer, srcPath, archivePath string, modTime time.Time) error {
	info, err := os.Stat(srcPath)
	if err != nil {
		return err
	}
	mode := int64(0o644)
	if info.Mode()&0o111 != 0 {
		mode = 0o755
	}
	header := &tar.Header{
		Name:     archivePath,
		Mode:     mode,
		Size:     info.Size(),
		ModTime:  modTime,
		Typeflag: tar.TypeReg,
		Format:   tar.FormatUSTAR,
		Uid:      0,
		Gid:      0,
		Uname:    "",
		Gname:    "",
	}
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	file, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(tw, file)
	return err
}

func addZipEntry(zw *zip.Writer, srcPath, archivePath string, modTime time.Time) error {
	info, err := os.Stat(srcPath)
	if err != nil {
		return err
	}
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = archivePath
	header.Method = zip.Deflate
	header.Modified = modTime
	if info.Mode()&0o111 != 0 {
		header.SetMode(0o755)
	} else {
		header.SetMode(0o644)
	}
	writer, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}
	file, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(writer, file)
	return err
}

func copyFile(srcPath, dstPath string, mode os.FileMode) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}

func writeChecksums(path string, artifacts []Artifact) error {
	var builder strings.Builder
	for _, artifact := range artifacts {
		builder.WriteString(artifact.SHA256)
		builder.WriteString("  ")
		builder.WriteString(artifact.Name)
		builder.WriteByte('\n')
	}
	return os.WriteFile(path, []byte(builder.String()), 0o644)
}

func writeManifest(path string, manifest Manifest) error {
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	return os.WriteFile(path, payload, 0o644)
}

func renderReleaseNotes(changelogPath, version string, artifacts []Artifact) (string, error) {
	sectionBody, sectionName, err := extractChangelogSection(changelogPath, version)
	if err != nil {
		return "", err
	}

	grouped := map[string][]string{}
	for _, artifact := range artifacts {
		grouped[artifact.Target.Binary] = append(grouped[artifact.Target.Binary], artifact.Target.GOOS+"/"+artifact.Target.GOARCH)
	}
	for _, targets := range grouped {
		sort.Strings(targets)
	}

	var builder strings.Builder
	builder.WriteString("# ClipHub ")
	builder.WriteString(version)
	builder.WriteString("\n\n")
	builder.WriteString("_Generated from `CHANGELOG.md` (`")
	builder.WriteString(sectionName)
	builder.WriteString("`)._\n\n")
	builder.WriteString("## Highlights\n\n")
	if sectionBody == "" {
		builder.WriteString("- No release highlights were found in `CHANGELOG.md`.\n")
	} else {
		builder.WriteString(sectionBody)
		if !strings.HasSuffix(sectionBody, "\n") {
			builder.WriteByte('\n')
		}
	}

	builder.WriteString("\n## Release Assets\n\n")
	for _, binary := range []string{"cliphub", "clipd", "tailclip"} {
		targets, ok := grouped[binary]
		if !ok {
			continue
		}
		builder.WriteString("- `")
		builder.WriteString(binary)
		builder.WriteString("`: `")
		builder.WriteString(strings.Join(targets, "`, `"))
		builder.WriteString("`\n")
	}
	builder.WriteString("- SHA256 checksums are published in `")
	builder.WriteString(checksumsFileName(version))
	builder.WriteString("`.\n")
	builder.WriteString("- Artifact metadata is published in `")
	builder.WriteString(manifestFileName(version))
	builder.WriteString("`.\n")
	return builder.String(), nil
}

func extractChangelogSection(changelogPath, version string) (string, string, error) {
	content, err := os.ReadFile(changelogPath)
	if err != nil {
		return "", "", err
	}

	sections := map[string][]string{}
	current := ""
	for _, line := range strings.Split(string(content), "\n") {
		if strings.HasPrefix(line, "## ") && !strings.HasPrefix(line, "### ") {
			current = strings.TrimSpace(strings.TrimPrefix(line, "## "))
			if _, exists := sections[current]; !exists {
				sections[current] = nil
			}
			continue
		}
		if current != "" {
			sections[current] = append(sections[current], line)
		}
	}

	candidates := []string{version, strings.TrimPrefix(version, "v"), "Unreleased"}
	for _, candidate := range candidates {
		if lines, ok := sections[candidate]; ok {
			return strings.TrimSpace(strings.Join(lines, "\n")), candidate, nil
		}
	}

	return "", "", fmt.Errorf("no CHANGELOG section found for %q or Unreleased", version)
}

type checksumEntry struct {
	Name string
	Sum  string
}

func readChecksums(path string) ([]checksumEntry, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var entries []checksumEntry
	for _, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid checksum line: %q", line)
		}
		entries = append(entries, checksumEntry{
			Sum:  parts[0],
			Name: parts[1],
		})
	}
	return entries, nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func runOutput(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func sanitizeVersion(version string) string {
	return strings.ReplaceAll(strings.TrimSpace(version), "/", "-")
}
