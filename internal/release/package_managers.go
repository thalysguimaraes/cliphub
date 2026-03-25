package release

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	packageManagerHomepage       = "https://github.com/thalysguimaraes/cliphub"
	packageManagerLicenseURL     = "https://github.com/thalysguimaraes/cliphub/blob/main/LICENSE"
	wingetPublisher              = "Thalys Guimaraes"
	wingetPublisherURL           = "https://github.com/thalysguimaraes"
	wingetManifestVersion        = "1.12.0"
	homebrewFormulaSyntaxChecker = "ruby"
)

type PackageManagerOptions struct {
	RepoRoot         string
	DistDir          string
	OutputDir        string
	Version          string
	ManifestRef      string
	ChecksumsRef     string
	ReleaseAssetBase string
}

type PackageManagerResult struct {
	Version          string
	PackageVersion   string
	OutputDir        string
	ManifestRef      string
	ChecksumsRef     string
	ManifestURL      string
	ChecksumsURL     string
	ReleaseAssetBase string
	ManifestSHA256   string
	HomebrewFiles    []string
	ScoopFiles       []string
	WingetFiles      []string
}

type packageSpec struct {
	Binary               string
	Description          string
	HomebrewTargets      []Target
	HomebrewTestCommand  string
	HomebrewTestExitCode int
	HomebrewTestExpect   string
	ScoopTarget          *Target
	WingetTarget         *Target
	WingetIdentifier     string
	WingetPackageName    string
	WingetMoniker        string
	WingetTags           []string
}

type releasePackageInputs struct {
	repoRoot         string
	distDir          string
	outputDir        string
	version          string
	packageVersion   string
	manifest         Manifest
	manifestBytes    []byte
	manifestSHA256   string
	manifestRef      string
	manifestURL      string
	checksumsRef     string
	checksumsURL     string
	releaseAssetBase string
	artifactsByKey   map[string]Artifact
}

type scoopManifest struct {
	Version      string                       `json:"version"`
	Description  string                       `json:"description"`
	Homepage     string                       `json:"homepage"`
	License      scoopLicense                 `json:"license"`
	Architecture map[string]scoopArchitecture `json:"architecture"`
	ExtractDir   string                       `json:"extract_dir"`
	Bin          string                       `json:"bin"`
}

type scoopLicense struct {
	Identifier string `json:"identifier"`
	URL        string `json:"url"`
}

type scoopArchitecture struct {
	URL  string `json:"url"`
	Hash string `json:"hash"`
}

type wingetVersionManifest struct {
	PackageIdentifier string `yaml:"PackageIdentifier"`
	PackageVersion    string `yaml:"PackageVersion"`
	DefaultLocale     string `yaml:"DefaultLocale"`
	ManifestType      string `yaml:"ManifestType"`
	ManifestVersion   string `yaml:"ManifestVersion"`
}

type wingetDefaultLocaleManifest struct {
	PackageIdentifier string   `yaml:"PackageIdentifier"`
	PackageVersion    string   `yaml:"PackageVersion"`
	PackageLocale     string   `yaml:"PackageLocale"`
	Publisher         string   `yaml:"Publisher"`
	PublisherURL      string   `yaml:"PublisherUrl"`
	PackageName       string   `yaml:"PackageName"`
	PackageURL        string   `yaml:"PackageUrl"`
	License           string   `yaml:"License"`
	LicenseURL        string   `yaml:"LicenseUrl"`
	ShortDescription  string   `yaml:"ShortDescription"`
	Moniker           string   `yaml:"Moniker,omitempty"`
	Tags              []string `yaml:"Tags,omitempty"`
	ManifestType      string   `yaml:"ManifestType"`
	ManifestVersion   string   `yaml:"ManifestVersion"`
}

type wingetInstallerManifest struct {
	PackageIdentifier string            `yaml:"PackageIdentifier"`
	PackageVersion    string            `yaml:"PackageVersion"`
	InstallerType     string            `yaml:"InstallerType"`
	ReleaseDate       string            `yaml:"ReleaseDate"`
	Installers        []wingetInstaller `yaml:"Installers"`
	ManifestType      string            `yaml:"ManifestType"`
	ManifestVersion   string            `yaml:"ManifestVersion"`
}

type wingetInstaller struct {
	Architecture         string                      `yaml:"Architecture"`
	NestedInstallerType  string                      `yaml:"NestedInstallerType"`
	NestedInstallerFiles []wingetNestedInstallerFile `yaml:"NestedInstallerFiles"`
	InstallerURL         string                      `yaml:"InstallerUrl"`
	InstallerSha256      string                      `yaml:"InstallerSha256"`
}

type wingetNestedInstallerFile struct {
	RelativeFilePath     string `yaml:"RelativeFilePath"`
	PortableCommandAlias string `yaml:"PortableCommandAlias"`
}

var packageSpecs = []packageSpec{
	{
		Binary:      "cliphub",
		Description: "Hub broker for ClipHub clipboard sync over Tailscale.",
		HomebrewTargets: []Target{
			{Binary: "cliphub", GOOS: "linux", GOARCH: "amd64"},
		},
		HomebrewTestCommand:  "#{bin}/cliphub -h 2>&1",
		HomebrewTestExitCode: 1,
		HomebrewTestExpect:   "listen address in dev mode",
	},
	{
		Binary:      "clipd",
		Description: "ClipHub desktop agent for clipboard sync over Tailscale.",
		HomebrewTargets: []Target{
			{Binary: "clipd", GOOS: "darwin", GOARCH: "amd64"},
			{Binary: "clipd", GOOS: "darwin", GOARCH: "arm64"},
			{Binary: "clipd", GOOS: "linux", GOARCH: "amd64"},
		},
		HomebrewTestCommand:  "#{bin}/clipd -h 2>&1",
		HomebrewTestExitCode: 1,
		HomebrewTestExpect:   "flag: help requested",
		ScoopTarget:          &Target{Binary: "clipd", GOOS: "windows", GOARCH: "amd64"},
		WingetTarget:         &Target{Binary: "clipd", GOOS: "windows", GOARCH: "amd64"},
		WingetIdentifier:     "ThalysGuimaraes.Clipd",
		WingetPackageName:    "ClipHub Agent (clipd)",
		WingetMoniker:        "clipd",
		WingetTags:           []string{"clipboard", "tailscale", "sync", "cli"},
	},
	{
		Binary:      "tailclip",
		Description: "ClipHub command-line client for clipboard sync over Tailscale.",
		HomebrewTargets: []Target{
			{Binary: "tailclip", GOOS: "darwin", GOARCH: "amd64"},
			{Binary: "tailclip", GOOS: "darwin", GOARCH: "arm64"},
			{Binary: "tailclip", GOOS: "linux", GOARCH: "amd64"},
		},
		HomebrewTestCommand:  "#{bin}/tailclip 2>&1",
		HomebrewTestExitCode: 1,
		HomebrewTestExpect:   "Usage: tailclip",
		ScoopTarget:          &Target{Binary: "tailclip", GOOS: "windows", GOARCH: "amd64"},
		WingetTarget:         &Target{Binary: "tailclip", GOOS: "windows", GOARCH: "amd64"},
		WingetIdentifier:     "ThalysGuimaraes.Tailclip",
		WingetPackageName:    "Tailclip",
		WingetMoniker:        "tailclip",
		WingetTags:           []string{"clipboard", "tailscale", "sync", "cli"},
	},
}

func BuildPackageManagers(ctx context.Context, opts PackageManagerOptions) (PackageManagerResult, error) {
	inputs, err := loadReleasePackageInputs(ctx, opts)
	if err != nil {
		return PackageManagerResult{}, err
	}
	if err := os.RemoveAll(inputs.outputDir); err != nil {
		return PackageManagerResult{}, err
	}
	if err := os.MkdirAll(inputs.outputDir, 0o755); err != nil {
		return PackageManagerResult{}, err
	}

	result := packageManagerResultFromInputs(inputs)

	for _, spec := range packageSpecs {
		if len(spec.HomebrewTargets) > 0 {
			formula, err := renderHomebrewFormula(inputs, spec)
			if err != nil {
				return PackageManagerResult{}, err
			}
			formulaPath := filepath.Join(inputs.outputDir, "homebrew", spec.Binary+".rb")
			if err := os.MkdirAll(filepath.Dir(formulaPath), 0o755); err != nil {
				return PackageManagerResult{}, err
			}
			if err := os.WriteFile(formulaPath, []byte(formula), 0o644); err != nil {
				return PackageManagerResult{}, err
			}
			result.HomebrewFiles = append(result.HomebrewFiles, formulaPath)
		}

		if spec.ScoopTarget != nil {
			scoopPayload, err := renderScoopManifest(inputs, spec)
			if err != nil {
				return PackageManagerResult{}, err
			}
			scoopPath := filepath.Join(inputs.outputDir, "scoop", spec.Binary+".json")
			if err := os.MkdirAll(filepath.Dir(scoopPath), 0o755); err != nil {
				return PackageManagerResult{}, err
			}
			if err := os.WriteFile(scoopPath, scoopPayload, 0o644); err != nil {
				return PackageManagerResult{}, err
			}
			result.ScoopFiles = append(result.ScoopFiles, scoopPath)
		}

		if spec.WingetTarget != nil {
			paths, err := writeWingetManifests(inputs, spec)
			if err != nil {
				return PackageManagerResult{}, err
			}
			result.WingetFiles = append(result.WingetFiles, paths...)
		}
	}

	sort.Strings(result.HomebrewFiles)
	sort.Strings(result.ScoopFiles)
	sort.Strings(result.WingetFiles)
	return result, nil
}

func VerifyPackageManagers(ctx context.Context, opts PackageManagerOptions) (PackageManagerResult, error) {
	inputs, err := loadReleasePackageInputs(ctx, opts)
	if err != nil {
		return PackageManagerResult{}, err
	}
	result := packageManagerResultFromInputs(inputs)

	for _, spec := range packageSpecs {
		if len(spec.HomebrewTargets) > 0 {
			formulaPath := filepath.Join(inputs.outputDir, "homebrew", spec.Binary+".rb")
			if err := verifyHomebrewFormula(ctx, inputs, spec, formulaPath); err != nil {
				return PackageManagerResult{}, err
			}
			result.HomebrewFiles = append(result.HomebrewFiles, formulaPath)
		}

		if spec.ScoopTarget != nil {
			scoopPath := filepath.Join(inputs.outputDir, "scoop", spec.Binary+".json")
			if err := verifyScoopManifest(inputs, spec, scoopPath); err != nil {
				return PackageManagerResult{}, err
			}
			result.ScoopFiles = append(result.ScoopFiles, scoopPath)
		}

		if spec.WingetTarget != nil {
			paths, err := verifyWingetManifests(inputs, spec)
			if err != nil {
				return PackageManagerResult{}, err
			}
			result.WingetFiles = append(result.WingetFiles, paths...)
		}
	}

	sort.Strings(result.HomebrewFiles)
	sort.Strings(result.ScoopFiles)
	sort.Strings(result.WingetFiles)
	return result, nil
}

func loadReleasePackageInputs(ctx context.Context, opts PackageManagerOptions) (releasePackageInputs, error) {
	repoRoot, err := normalizeRepoRoot(opts.RepoRoot)
	if err != nil {
		return releasePackageInputs{}, err
	}

	version := strings.TrimSpace(opts.Version)
	if version == "" {
		return releasePackageInputs{}, fmt.Errorf("version is required")
	}

	distDir := opts.DistDir
	if distDir == "" {
		distDir = filepath.Join(repoRoot, "dist", "release")
	}
	if !filepath.IsAbs(distDir) {
		distDir = filepath.Join(repoRoot, distDir)
	}

	manifestRef := strings.TrimSpace(opts.ManifestRef)
	if manifestRef == "" {
		manifestRef = filepath.Join(distDir, manifestFileName(version))
	}
	checksumsRef := strings.TrimSpace(opts.ChecksumsRef)
	if checksumsRef == "" {
		checksumsRef = filepath.Join(distDir, checksumsFileName(version))
	}

	manifestBytes, manifestRemote, err := readReleaseRef(ctx, manifestRef)
	if err != nil {
		return releasePackageInputs{}, err
	}
	checksumBytes, checksumsRemote, err := readReleaseRef(ctx, checksumsRef)
	if err != nil {
		return releasePackageInputs{}, err
	}

	var manifest Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return releasePackageInputs{}, fmt.Errorf("read manifest: %w", err)
	}
	if strings.TrimSpace(manifest.Version) == "" {
		return releasePackageInputs{}, fmt.Errorf("manifest version is required")
	}
	if manifest.Version != version {
		return releasePackageInputs{}, fmt.Errorf("manifest version mismatch: expected %s, got %s", version, manifest.Version)
	}

	checksumEntries, err := parseChecksums(checksumBytes)
	if err != nil {
		return releasePackageInputs{}, err
	}
	if err := validateManifestChecksums(manifest, checksumEntries); err != nil {
		return releasePackageInputs{}, err
	}

	manifestSHA := sha256.Sum256(manifestBytes)
	artifactBase := strings.TrimRight(strings.TrimSpace(opts.ReleaseAssetBase), "/")
	if artifactBase == "" {
		switch {
		case manifestRemote:
			artifactBase, err = releaseAssetBaseFromURL(manifestRef)
		case checksumsRemote:
			artifactBase, err = releaseAssetBaseFromURL(checksumsRef)
		default:
			err = fmt.Errorf("release asset base URL is required when manifest/checksum inputs are local paths")
		}
		if err != nil {
			return releasePackageInputs{}, err
		}
	}

	manifestURL, err := releaseAssetURL(manifestRef, manifestRemote, artifactBase)
	if err != nil {
		return releasePackageInputs{}, err
	}
	checksumsURL, err := releaseAssetURL(checksumsRef, checksumsRemote, artifactBase)
	if err != nil {
		return releasePackageInputs{}, err
	}

	outputDir := strings.TrimSpace(opts.OutputDir)
	if outputDir == "" {
		outputDir = filepath.Join(distDir, "package-managers")
	}
	if !filepath.IsAbs(outputDir) {
		outputDir = filepath.Join(repoRoot, outputDir)
	}

	artifactsByKey := make(map[string]Artifact, len(manifest.Artifacts))
	for _, artifact := range manifest.Artifacts {
		artifactsByKey[artifactKey(artifact.Target)] = artifact
	}

	return releasePackageInputs{
		repoRoot:         repoRoot,
		distDir:          distDir,
		outputDir:        outputDir,
		version:          version,
		packageVersion:   strings.TrimPrefix(version, "v"),
		manifest:         manifest,
		manifestBytes:    manifestBytes,
		manifestSHA256:   hex.EncodeToString(manifestSHA[:]),
		manifestRef:      manifestRef,
		manifestURL:      manifestURL,
		checksumsRef:     checksumsRef,
		checksumsURL:     checksumsURL,
		releaseAssetBase: artifactBase,
		artifactsByKey:   artifactsByKey,
	}, nil
}

func packageManagerResultFromInputs(inputs releasePackageInputs) PackageManagerResult {
	return PackageManagerResult{
		Version:          inputs.version,
		PackageVersion:   inputs.packageVersion,
		OutputDir:        inputs.outputDir,
		ManifestRef:      inputs.manifestRef,
		ChecksumsRef:     inputs.checksumsRef,
		ManifestURL:      inputs.manifestURL,
		ChecksumsURL:     inputs.checksumsURL,
		ReleaseAssetBase: inputs.releaseAssetBase,
		ManifestSHA256:   inputs.manifestSHA256,
	}
}

func renderHomebrewFormula(inputs releasePackageInputs, spec packageSpec) (string, error) {
	var builder strings.Builder
	builder.WriteString("class ")
	builder.WriteString(homebrewClassName(spec.Binary))
	builder.WriteString(" < Formula\n")
	builder.WriteString("  desc ")
	builder.WriteString(rubyString(spec.Description))
	builder.WriteString("\n")
	builder.WriteString("  homepage ")
	builder.WriteString(rubyString(packageManagerHomepage))
	builder.WriteString("\n")
	builder.WriteString("  url ")
	builder.WriteString(rubyString(inputs.manifestURL))
	builder.WriteString("\n")
	builder.WriteString("  sha256 ")
	builder.WriteString(rubyString(inputs.manifestSHA256))
	builder.WriteString("\n")
	builder.WriteString("  version ")
	builder.WriteString(rubyString(inputs.packageVersion))
	builder.WriteString("\n")
	builder.WriteString("  license \"MIT\"\n\n")

	if spec.Binary == "cliphub" {
		builder.WriteString("  depends_on :linux\n")
		builder.WriteString("  depends_on arch: :x86_64\n\n")
	} else {
		builder.WriteString("  on_linux do\n")
		builder.WriteString("    depends_on arch: :x86_64\n")
		builder.WriteString("  end\n\n")
	}

	builder.WriteString("  resource \"archive\" do\n")
	if err := writeHomebrewResourceBlocks(&builder, inputs, spec); err != nil {
		return "", err
	}
	builder.WriteString("  end\n\n")

	builder.WriteString("  def install\n")
	builder.WriteString("    resource(\"archive\").stage do\n")
	builder.WriteString("      root = Dir[\"*\"].find { |entry| File.directory?(entry) } || \".\"\n")
	builder.WriteString("      bin.install File.join(root, ")
	builder.WriteString(rubyString(spec.Binary))
	builder.WriteString(")\n")
	builder.WriteString("      doc.install File.join(root, \"README.md\") if File.exist?(File.join(root, \"README.md\"))\n")
	builder.WriteString("      license.install File.join(root, \"LICENSE\") if File.exist?(File.join(root, \"LICENSE\"))\n")
	builder.WriteString("    end\n")
	builder.WriteString("  end\n\n")

	builder.WriteString("  test do\n")
	builder.WriteString("    assert_match ")
	builder.WriteString(rubyString(spec.HomebrewTestExpect))
	builder.WriteString(", shell_output(")
	builder.WriteString(rubyString(spec.HomebrewTestCommand))
	builder.WriteString(", ")
	builder.WriteString(fmt.Sprintf("%d", spec.HomebrewTestExitCode))
	builder.WriteString(")\n")
	builder.WriteString("  end\n")
	builder.WriteString("end\n")
	return builder.String(), nil
}

func writeHomebrewResourceBlocks(builder *strings.Builder, inputs releasePackageInputs, spec packageSpec) error {
	armDarwin, hasArmDarwin := inputs.artifact(spec.Binary, "darwin", "arm64")
	intelDarwin, hasIntelDarwin := inputs.artifact(spec.Binary, "darwin", "amd64")
	intelLinux, hasIntelLinux := inputs.artifact(spec.Binary, "linux", "amd64")

	if hasArmDarwin {
		builder.WriteString("    on_arm do\n")
		builder.WriteString("      on_macos do\n")
		builder.WriteString("        url ")
		builder.WriteString(rubyString(inputs.assetURL(armDarwin)))
		builder.WriteString("\n")
		builder.WriteString("        sha256 ")
		builder.WriteString(rubyString(armDarwin.SHA256))
		builder.WriteString("\n")
		builder.WriteString("      end\n")
		builder.WriteString("    end\n")
	}
	if hasIntelDarwin || hasIntelLinux {
		builder.WriteString("    on_intel do\n")
		if hasIntelDarwin {
			builder.WriteString("      on_macos do\n")
			builder.WriteString("        url ")
			builder.WriteString(rubyString(inputs.assetURL(intelDarwin)))
			builder.WriteString("\n")
			builder.WriteString("        sha256 ")
			builder.WriteString(rubyString(intelDarwin.SHA256))
			builder.WriteString("\n")
			builder.WriteString("      end\n")
		}
		if hasIntelLinux {
			builder.WriteString("      on_linux do\n")
			builder.WriteString("        url ")
			builder.WriteString(rubyString(inputs.assetURL(intelLinux)))
			builder.WriteString("\n")
			builder.WriteString("        sha256 ")
			builder.WriteString(rubyString(intelLinux.SHA256))
			builder.WriteString("\n")
			builder.WriteString("      end\n")
		}
		builder.WriteString("    end\n")
	}

	if !hasArmDarwin && !hasIntelDarwin && !hasIntelLinux {
		return fmt.Errorf("no Homebrew artifacts available for %s", spec.Binary)
	}
	return nil
}

func renderScoopManifest(inputs releasePackageInputs, spec packageSpec) ([]byte, error) {
	artifact, err := inputs.requiredArtifact(*spec.ScoopTarget)
	if err != nil {
		return nil, err
	}
	payload := scoopManifest{
		Version:     inputs.packageVersion,
		Description: spec.Description,
		Homepage:    packageManagerHomepage,
		License: scoopLicense{
			Identifier: "MIT",
			URL:        packageManagerLicenseURL,
		},
		Architecture: map[string]scoopArchitecture{
			"64bit": {
				URL:  inputs.assetURL(artifact),
				Hash: artifact.SHA256,
			},
		},
		ExtractDir: archiveRootDir(artifact),
		Bin:        spec.WingetMoniker + ".exe",
	}
	return json.MarshalIndent(payload, "", "  ")
}

func writeWingetManifests(inputs releasePackageInputs, spec packageSpec) ([]string, error) {
	artifact, err := inputs.requiredArtifact(*spec.WingetTarget)
	if err != nil {
		return nil, err
	}

	dir := wingetManifestDir(inputs.outputDir, spec.WingetIdentifier, inputs.packageVersion)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	versionPayload := wingetVersionManifest{
		PackageIdentifier: spec.WingetIdentifier,
		PackageVersion:    inputs.packageVersion,
		DefaultLocale:     "en-US",
		ManifestType:      "version",
		ManifestVersion:   wingetManifestVersion,
	}
	defaultLocalePayload := wingetDefaultLocaleManifest{
		PackageIdentifier: spec.WingetIdentifier,
		PackageVersion:    inputs.packageVersion,
		PackageLocale:     "en-US",
		Publisher:         wingetPublisher,
		PublisherURL:      wingetPublisherURL,
		PackageName:       spec.WingetPackageName,
		PackageURL:        packageManagerHomepage,
		License:           "MIT",
		LicenseURL:        packageManagerLicenseURL,
		ShortDescription:  spec.Description,
		Moniker:           spec.WingetMoniker,
		Tags:              spec.WingetTags,
		ManifestType:      "defaultLocale",
		ManifestVersion:   wingetManifestVersion,
	}
	installerPayload := wingetInstallerManifest{
		PackageIdentifier: spec.WingetIdentifier,
		PackageVersion:    inputs.packageVersion,
		InstallerType:     "zip",
		ReleaseDate:       time.Unix(inputs.manifest.SourceDateEpoch, 0).UTC().Format("2006-01-02"),
		Installers: []wingetInstaller{
			{
				Architecture:        "x64",
				NestedInstallerType: "portable",
				NestedInstallerFiles: []wingetNestedInstallerFile{
					{
						RelativeFilePath:     archiveRootDir(artifact) + `\` + spec.WingetMoniker + ".exe",
						PortableCommandAlias: spec.WingetMoniker,
					},
				},
				InstallerURL:    inputs.assetURL(artifact),
				InstallerSha256: strings.ToUpper(artifact.SHA256),
			},
		},
		ManifestType:    "installer",
		ManifestVersion: wingetManifestVersion,
	}

	files := []struct {
		name    string
		payload any
	}{
		{name: spec.WingetIdentifier + ".yaml", payload: versionPayload},
		{name: spec.WingetIdentifier + ".locale.en-US.yaml", payload: defaultLocalePayload},
		{name: spec.WingetIdentifier + ".installer.yaml", payload: installerPayload},
	}

	var written []string
	for _, file := range files {
		dst := filepath.Join(dir, file.name)
		if err := writeWingetYAML(dst, file.payload); err != nil {
			return nil, err
		}
		written = append(written, dst)
	}
	return written, nil
}

func verifyHomebrewFormula(ctx context.Context, inputs releasePackageInputs, spec packageSpec, formulaPath string) error {
	content, err := os.ReadFile(formulaPath)
	if err != nil {
		return err
	}
	expected, err := renderHomebrewFormula(inputs, spec)
	if err != nil {
		return err
	}
	if string(content) != expected {
		return fmt.Errorf("homebrew formula mismatch: %s", formulaPath)
	}

	if _, err := exec.LookPath(homebrewFormulaSyntaxChecker); err == nil {
		cmd := exec.CommandContext(ctx, homebrewFormulaSyntaxChecker, "-c", formulaPath)
		output, runErr := cmd.CombinedOutput()
		if runErr != nil {
			return fmt.Errorf("ruby syntax check failed for %s: %s", formulaPath, strings.TrimSpace(string(output)))
		}
	}
	return nil
}

func verifyScoopManifest(inputs releasePackageInputs, spec packageSpec, scoopPath string) error {
	content, err := os.ReadFile(scoopPath)
	if err != nil {
		return err
	}
	var got scoopManifest
	if err := json.Unmarshal(content, &got); err != nil {
		return fmt.Errorf("parse scoop manifest %s: %w", scoopPath, err)
	}
	expectedBytes, err := renderScoopManifest(inputs, spec)
	if err != nil {
		return err
	}
	var want scoopManifest
	if err := json.Unmarshal(expectedBytes, &want); err != nil {
		return err
	}
	if !scoopManifestsEqual(got, want) {
		return fmt.Errorf("scoop manifest mismatch: %s", scoopPath)
	}
	return nil
}

func verifyWingetManifests(inputs releasePackageInputs, spec packageSpec) ([]string, error) {
	dir := wingetManifestDir(inputs.outputDir, spec.WingetIdentifier, inputs.packageVersion)
	versionPath := filepath.Join(dir, spec.WingetIdentifier+".yaml")
	defaultLocalePath := filepath.Join(dir, spec.WingetIdentifier+".locale.en-US.yaml")
	installerPath := filepath.Join(dir, spec.WingetIdentifier+".installer.yaml")

	artifact, err := inputs.requiredArtifact(*spec.WingetTarget)
	if err != nil {
		return nil, err
	}

	var versionManifest wingetVersionManifest
	if err := readWingetYAML(versionPath, &versionManifest); err != nil {
		return nil, err
	}
	if versionManifest != (wingetVersionManifest{
		PackageIdentifier: spec.WingetIdentifier,
		PackageVersion:    inputs.packageVersion,
		DefaultLocale:     "en-US",
		ManifestType:      "version",
		ManifestVersion:   wingetManifestVersion,
	}) {
		return nil, fmt.Errorf("winget version manifest mismatch: %s", versionPath)
	}

	var defaultLocale wingetDefaultLocaleManifest
	if err := readWingetYAML(defaultLocalePath, &defaultLocale); err != nil {
		return nil, err
	}
	wantDefaultLocale := wingetDefaultLocaleManifest{
		PackageIdentifier: spec.WingetIdentifier,
		PackageVersion:    inputs.packageVersion,
		PackageLocale:     "en-US",
		Publisher:         wingetPublisher,
		PublisherURL:      wingetPublisherURL,
		PackageName:       spec.WingetPackageName,
		PackageURL:        packageManagerHomepage,
		License:           "MIT",
		LicenseURL:        packageManagerLicenseURL,
		ShortDescription:  spec.Description,
		Moniker:           spec.WingetMoniker,
		Tags:              spec.WingetTags,
		ManifestType:      "defaultLocale",
		ManifestVersion:   wingetManifestVersion,
	}
	if !wingetDefaultLocaleEqual(defaultLocale, wantDefaultLocale) {
		return nil, fmt.Errorf("winget default locale manifest mismatch: %s", defaultLocalePath)
	}

	var installer wingetInstallerManifest
	if err := readWingetYAML(installerPath, &installer); err != nil {
		return nil, err
	}
	wantInstaller := wingetInstallerManifest{
		PackageIdentifier: spec.WingetIdentifier,
		PackageVersion:    inputs.packageVersion,
		InstallerType:     "zip",
		ReleaseDate:       time.Unix(inputs.manifest.SourceDateEpoch, 0).UTC().Format("2006-01-02"),
		Installers: []wingetInstaller{
			{
				Architecture:        "x64",
				NestedInstallerType: "portable",
				NestedInstallerFiles: []wingetNestedInstallerFile{
					{
						RelativeFilePath:     archiveRootDir(artifact) + `\` + spec.WingetMoniker + ".exe",
						PortableCommandAlias: spec.WingetMoniker,
					},
				},
				InstallerURL:    inputs.assetURL(artifact),
				InstallerSha256: strings.ToUpper(artifact.SHA256),
			},
		},
		ManifestType:    "installer",
		ManifestVersion: wingetManifestVersion,
	}
	if !wingetInstallerEqual(installer, wantInstaller) {
		return nil, fmt.Errorf("winget installer manifest mismatch: %s", installerPath)
	}

	return []string{defaultLocalePath, installerPath, versionPath}, nil
}

func writeWingetYAML(path string, payload any) error {
	content, err := yaml.Marshal(payload)
	if err != nil {
		return err
	}

	schemaURL := "https://aka.ms/winget-manifest.installer." + wingetManifestVersion + ".schema.json"
	switch payload.(type) {
	case wingetVersionManifest:
		schemaURL = "https://aka.ms/winget-manifest.version." + wingetManifestVersion + ".schema.json"
	case wingetDefaultLocaleManifest:
		schemaURL = "https://aka.ms/winget-manifest.defaultLocale." + wingetManifestVersion + ".schema.json"
	}

	var builder strings.Builder
	builder.WriteString("# yaml-language-server: $schema=")
	builder.WriteString(schemaURL)
	builder.WriteString("\n\n")
	builder.Write(content)
	return os.WriteFile(path, []byte(builder.String()), 0o644)
}

func readWingetYAML(path string, target any) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(content), "\n")
	start := 0
	for start < len(lines) && strings.HasPrefix(lines[start], "#") {
		start++
	}
	payload := strings.TrimSpace(strings.Join(lines[start:], "\n"))
	if payload == "" {
		return fmt.Errorf("winget manifest is empty: %s", path)
	}
	return yaml.Unmarshal([]byte(payload), target)
}

func parseChecksums(content []byte) ([]checksumEntry, error) {
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

func validateManifestChecksums(manifest Manifest, entries []checksumEntry) error {
	if len(entries) != len(manifest.Artifacts) {
		return fmt.Errorf("artifact count mismatch: checksums=%d manifest=%d", len(entries), len(manifest.Artifacts))
	}
	for idx, entry := range entries {
		artifact := manifest.Artifacts[idx]
		if artifact.Name != entry.Name {
			return fmt.Errorf("manifest order mismatch at %d: checksum=%s manifest=%s", idx, entry.Name, artifact.Name)
		}
		if artifact.SHA256 != entry.Sum {
			return fmt.Errorf("manifest checksum mismatch for %s", artifact.Name)
		}
	}
	return nil
}

func readReleaseRef(ctx context.Context, ref string) ([]byte, bool, error) {
	if isRemoteReleaseRef(ref) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, ref, nil)
		if err != nil {
			return nil, true, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, true, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, true, fmt.Errorf("download %s: unexpected HTTP %d", ref, resp.StatusCode)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, true, err
		}
		return body, true, nil
	}
	body, err := os.ReadFile(ref)
	return body, false, err
}

func releaseAssetBaseFromURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	parsed.Path = strings.TrimRight(path.Dir(parsed.Path), "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func releaseAssetURL(ref string, isRemote bool, releaseAssetBase string) (string, error) {
	if isRemote {
		return ref, nil
	}
	base, err := url.Parse(strings.TrimRight(releaseAssetBase, "/") + "/")
	if err != nil {
		return "", err
	}
	base.Path = path.Join(base.Path, filepath.Base(ref))
	return base.String(), nil
}

func isRemoteReleaseRef(ref string) bool {
	parsed, err := url.Parse(ref)
	if err != nil {
		return false
	}
	switch parsed.Scheme {
	case "http", "https":
		return parsed.Host != ""
	default:
		return false
	}
}

func (inputs releasePackageInputs) requiredArtifact(target Target) (Artifact, error) {
	artifact, ok := inputs.artifact(target.Binary, target.GOOS, target.GOARCH)
	if !ok {
		return Artifact{}, fmt.Errorf("release artifact missing for %s %s/%s", target.Binary, target.GOOS, target.GOARCH)
	}
	return artifact, nil
}

func (inputs releasePackageInputs) artifact(binary, goos, goarch string) (Artifact, bool) {
	artifact, ok := inputs.artifactsByKey[artifactKey(Target{
		Binary: binary,
		GOOS:   goos,
		GOARCH: goarch,
	})]
	return artifact, ok
}

func (inputs releasePackageInputs) assetURL(artifact Artifact) string {
	base, _ := url.Parse(strings.TrimRight(inputs.releaseAssetBase, "/") + "/")
	base.Path = path.Join(base.Path, artifact.Path)
	return base.String()
}

func artifactKey(target Target) string {
	return strings.Join([]string{target.Binary, target.GOOS, target.GOARCH}, "|")
}

func archiveRootDir(artifact Artifact) string {
	root := artifact.Name
	switch artifact.Format {
	case "tar.gz":
		root = strings.TrimSuffix(root, ".tar.gz")
	case "zip":
		root = strings.TrimSuffix(root, ".zip")
	default:
		root = strings.TrimSuffix(root, filepath.Ext(root))
	}
	return root
}

func wingetManifestDir(root, identifier, version string) string {
	parts := strings.Split(identifier, ".")
	if len(parts) != 2 {
		return filepath.Join(root, "winget", "manifests", version)
	}
	return filepath.Join(
		root,
		"winget",
		"manifests",
		strings.ToLower(parts[0][:1]),
		parts[0],
		parts[1],
		version,
	)
}

func homebrewClassName(binary string) string {
	return strings.ToUpper(binary[:1]) + binary[1:]
}

func rubyString(value string) string {
	return fmt.Sprintf("%q", value)
}

func scoopManifestsEqual(a, b scoopManifest) bool {
	if a.Version != b.Version || a.Description != b.Description || a.Homepage != b.Homepage || a.ExtractDir != b.ExtractDir || a.Bin != b.Bin {
		return false
	}
	if a.License != b.License {
		return false
	}
	return a.Architecture["64bit"] == b.Architecture["64bit"]
}

func wingetDefaultLocaleEqual(a, b wingetDefaultLocaleManifest) bool {
	if a.PackageIdentifier != b.PackageIdentifier || a.PackageVersion != b.PackageVersion || a.PackageLocale != b.PackageLocale || a.Publisher != b.Publisher || a.PublisherURL != b.PublisherURL || a.PackageName != b.PackageName || a.PackageURL != b.PackageURL || a.License != b.License || a.LicenseURL != b.LicenseURL || a.ShortDescription != b.ShortDescription || a.Moniker != b.Moniker || a.ManifestType != b.ManifestType || a.ManifestVersion != b.ManifestVersion {
		return false
	}
	if len(a.Tags) != len(b.Tags) {
		return false
	}
	for idx := range a.Tags {
		if a.Tags[idx] != b.Tags[idx] {
			return false
		}
	}
	return true
}

func wingetInstallerEqual(a, b wingetInstallerManifest) bool {
	if a.PackageIdentifier != b.PackageIdentifier || a.PackageVersion != b.PackageVersion || a.InstallerType != b.InstallerType || a.ReleaseDate != b.ReleaseDate || a.ManifestType != b.ManifestType || a.ManifestVersion != b.ManifestVersion {
		return false
	}
	if len(a.Installers) != len(b.Installers) {
		return false
	}
	for idx := range a.Installers {
		if !wingetInstallerEntryEqual(a.Installers[idx], b.Installers[idx]) {
			return false
		}
	}
	return true
}

func wingetInstallerEntryEqual(a, b wingetInstaller) bool {
	if a.Architecture != b.Architecture || a.NestedInstallerType != b.NestedInstallerType || a.InstallerURL != b.InstallerURL || a.InstallerSha256 != b.InstallerSha256 {
		return false
	}
	if len(a.NestedInstallerFiles) != len(b.NestedInstallerFiles) {
		return false
	}
	for idx := range a.NestedInstallerFiles {
		if a.NestedInstallerFiles[idx] != b.NestedInstallerFiles[idx] {
			return false
		}
	}
	return true
}
