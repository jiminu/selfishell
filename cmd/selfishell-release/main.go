package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/jiminu/selfishell/internal/releasebuild"
	"github.com/jiminu/selfishell/internal/selfishell"
)

const usage = "Usage: scripts/build-release.sh --version VERSION [--output OUTPUT_DIR]"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	root := os.Args[1]
	args := os.Args[2:]
	version, output := "", ""
	for len(args) > 0 {
		if args[0] != "--version" && args[0] != "--output" {
			fmt.Fprintln(os.Stderr, "Unknown option:", args[0])
			fmt.Fprintln(os.Stderr, usage)
			os.Exit(2)
		}
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, usage)
			os.Exit(2)
		}
		if args[0] == "--version" {
			version = args[1]
		} else {
			output = args[1]
		}
		args = args[2:]
	}
	if !selfishell.ValidReleaseVersion(version) {
		fmt.Fprintln(os.Stderr, "A valid semantic version is required.")
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := releasebuild.Build(ctx, root, version, output); err != nil {
		fmt.Fprintln(os.Stderr, "Build failed:", err)
		os.Exit(1)
	}
	fmt.Printf("Built Selfishell %s native release artifacts\n", version)
}
