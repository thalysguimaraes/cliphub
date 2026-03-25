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

func usageError(command string) error {
	if command == "" {
		return fmt.Errorf("usage: releasectl <build|verify> [flags]")
	}
	return fmt.Errorf("unknown command %q (usage: releasectl <build|verify> [flags])", command)
}
