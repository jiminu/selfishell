package main

import (
	"fmt"
	"os"

	"github.com/jiminu/selfishell/internal/benchmark"
)

func main() {
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "benchmark:", err)
		os.Exit(1)
	}
	os.Exit(benchmark.Run(os.Args[1:], os.Environ(), root, os.Stdout, os.Stderr))
}
