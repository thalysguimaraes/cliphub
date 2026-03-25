package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/thalysguimaraes/cliphub/internal/release"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "releasectl: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return usageError("")
	}

	switch args[0] {
	case "build":
		return runBuild(ctx, args[1:])
	case "package-managers":
		return runPackageManagers(ctx, args[1:])
	case "verify":
		return runVerify(args[1:])
	default:
		return usageError(args[0])
	}
}

func runBuild(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("releasectl build", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	repoRoot := fs.String("repo-root", ".", "repository root")
	distDir := fs.String("dist", filepath.Join("dist", "release"), "output directory")
	version := fs.String("version", "", "release version or tag")
	sourceDateEpoch := fs.Int64("source-date-epoch", 0, "override SOURCE_DATE_EPOCH for deterministic archives")
	if err := fs.Parse(args); err != nil {
		return err
	}

	resolvedVersion := *version
	if resolvedVersion == "" {
		var err error
		resolvedVersion, err = release.ResolveVersion(ctx, *repoRoot)
		if err != nil {
			return err
		}
	}

	result, err := release.Build(ctx, release.Options{
		RepoRoot:        *repoRoot,
		DistDir:         *distDir,
		Version:         resolvedVersion,
		SourceDateEpoch: *sourceDateEpoch,
	})
	if err != nil {
		return err
	}

	fmt.Printf("built %d release archives in %s\n", len(result.Artifacts), result.DistDir)
	fmt.Printf("source date epoch: %d (%s)\n", result.SourceDateEpoch, time.Unix(result.SourceDateEpoch, 0).UTC().Format(time.RFC3339))
	fmt.Printf("checksums: %s\n", result.ChecksumsPath)
	fmt.Printf("release notes: %s\n", result.NotesPath)
	fmt.Printf("manifest: %s\n", result.ManifestPath)
	return nil
}

func runVerify(args []string) error {
	fs := flag.NewFlagSet("releasectl verify", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	repoRoot := fs.String("repo-root", ".", "repository root")
	distDir := fs.String("dist", filepath.Join("dist", "release"), "output directory")
	version := fs.String("version", "", "release version or tag")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *version == "" {
		return fmt.Errorf("version is required for verify")
	}

	result, err := release.Verify(release.VerifyOptions{
		RepoRoot: *repoRoot,
		DistDir:  *distDir,
		Version:  *version,
	})
	if err != nil {
		return err
	}

	fmt.Printf("verified %d release archives from %s\n", len(result.Artifacts), result.DistDir)
	return nil
}

func runPackageManagers(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: releasectl package-managers <build|verify> [flags]")
	}

	switch args[0] {
	case "build":
		return runPackageManagersBuild(ctx, args[1:])
	case "verify":
		return runPackageManagersVerify(ctx, args[1:])
	default:
		return fmt.Errorf("unknown package-managers command %q (usage: releasectl package-managers <build|verify> [flags])", args[0])
	}
}

func runPackageManagersBuild(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("releasectl package-managers build", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	repoRoot := fs.String("repo-root", ".", "repository root")
	distDir := fs.String("dist", filepath.Join("dist", "release"), "release artifact directory")
	outputDir := fs.String("out", "", "package-manager output directory (default: <dist>/package-managers)")
	version := fs.String("version", "", "release version or tag")
	manifestRef := fs.String("manifest", "", "release manifest path or URL")
	checksumsRef := fs.String("checksums", "", "release checksums path or URL")
	releaseAssetBase := fs.String("release-asset-base", "", "base URL used by generated package-manager metadata for release assets")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *version == "" {
		return fmt.Errorf("version is required for package-managers build")
	}

	result, err := release.BuildPackageManagers(ctx, release.PackageManagerOptions{
		RepoRoot:         *repoRoot,
		DistDir:          *distDir,
		OutputDir:        *outputDir,
		Version:          *version,
		ManifestRef:      *manifestRef,
		ChecksumsRef:     *checksumsRef,
		ReleaseAssetBase: *releaseAssetBase,
	})
	if err != nil {
		return err
	}

	fmt.Printf("generated %d package-manager files in %s\n", len(result.HomebrewFiles)+len(result.ScoopFiles)+len(result.WingetFiles), result.OutputDir)
	fmt.Printf("homebrew: %d\n", len(result.HomebrewFiles))
	fmt.Printf("scoop: %d\n", len(result.ScoopFiles))
	fmt.Printf("winget: %d\n", len(result.WingetFiles))
	fmt.Printf("manifest: %s\n", result.ManifestRef)
	fmt.Printf("checksums: %s\n", result.ChecksumsRef)
	return nil
}

func runPackageManagersVerify(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("releasectl package-managers verify", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	repoRoot := fs.String("repo-root", ".", "repository root")
	distDir := fs.String("dist", filepath.Join("dist", "release"), "release artifact directory")
	outputDir := fs.String("out", "", "package-manager output directory (default: <dist>/package-managers)")
	version := fs.String("version", "", "release version or tag")
	manifestRef := fs.String("manifest", "", "release manifest path or URL")
	checksumsRef := fs.String("checksums", "", "release checksums path or URL")
	releaseAssetBase := fs.String("release-asset-base", "", "base URL used by generated package-manager metadata for release assets")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *version == "" {
		return fmt.Errorf("version is required for package-managers verify")
	}

	result, err := release.VerifyPackageManagers(ctx, release.PackageManagerOptions{
		RepoRoot:         *repoRoot,
		DistDir:          *distDir,
		OutputDir:        *outputDir,
		Version:          *version,
		ManifestRef:      *manifestRef,
		ChecksumsRef:     *checksumsRef,
		ReleaseAssetBase: *releaseAssetBase,
	})
	if err != nil {
		return err
	}

	fmt.Printf("verified %d package-manager files in %s\n", len(result.HomebrewFiles)+len(result.ScoopFiles)+len(result.WingetFiles), result.OutputDir)
	return nil
}

func usageError(command string) error {
	if command == "" {
		return fmt.Errorf("usage: releasectl <build|verify|package-managers> [flags]")
	}
	return fmt.Errorf("unknown command %q (usage: releasectl <build|verify|package-managers> [flags])", command)
}
