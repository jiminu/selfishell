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
	if len(os.Args) >= 3 && os.Args[2] == "update-dependencies" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		p := selfishell.Process{In: os.Stdin, Out: os.Stdout, Err: os.Stderr, Env: os.Environ()}
		os.Exit(runDependencyUpdate(ctx, os.Args[1], os.Args[3:], p))
	}
	if len(os.Args) >= 2 && os.Args[1] == "curl" {
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "usage: selfishell-dev curl metadata|transfer <curl-args...>")
			os.Exit(2)
		}
		mode := os.Args[2]
		if mode != "metadata" && mode != "transfer" {
			fmt.Fprintln(os.Stderr, "selfishell-dev: unknown curl mode")
			os.Exit(2)
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		p := selfishell.Process{In: os.Stdin, Out: os.Stdout, Err: os.Stderr, Env: os.Environ()}
		status, err := p.Curl(ctx, mode, os.Args[3:]...)
		if err != nil {
			fmt.Fprintln(os.Stderr, "selfishell-dev:", err)
		}
		os.Exit(status)
	}
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: selfishell-dev <release-root> mise-only|benchmark-shell|neovim-e2e|update-dependencies [options] | curl metadata|transfer <curl-args...>")
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
