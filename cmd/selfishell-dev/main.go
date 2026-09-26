package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/jiminu/selfishell/internal/selfishell"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: selfishell-dev <release-root> mise-only|benchmark-shell|neovim-e2e")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	p := selfishell.Process{In: os.Stdin, Out: os.Stdout, Err: os.Stderr, Env: selfishell.DeveloperToolEnv(os.Args[1])}
	if err := selfishell.ProvisionDeveloper(ctx, os.Args[1], os.Args[2], p); err != nil {
		fmt.Fprintln(os.Stderr, "selfishell-dev:", err)
		os.Exit(1)
	}
}
