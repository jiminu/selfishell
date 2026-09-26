package main

import (
	"fmt"
	"os"

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
	cli := selfishell.CLI{Root: root, In: os.Stdin, Out: os.Stdout, Err: os.Stderr}
	os.Exit(cli.Run(os.Args[1:]))
}
