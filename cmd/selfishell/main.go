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
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "selfishell: Cannot locate executable:", err)
		os.Exit(1)
	}
	root, err := selfishell.ReleaseRoot(executable)
	if err != nil {
		fmt.Fprintln(os.Stderr, "selfishell: Cannot locate release:", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cli := selfishell.CLI{Root: root, In: os.Stdin, Out: os.Stdout, Err: os.Stderr, Context: ctx}
	os.Exit(cli.Run(os.Args[1:]))
}
