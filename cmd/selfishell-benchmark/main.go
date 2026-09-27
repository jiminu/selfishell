package main

import (
	"os"
	"path/filepath"

	"github.com/jiminu/selfishell/internal/benchmark"
)

func main() {
	source := os.Getenv("SELFISHELL_BENCHMARK_SOURCE_ROOT")
	if source == "" {
		var err error
		source, err = os.Getwd()
		if err != nil {
			panic(err)
		}
		source, err = filepath.Abs(source)
		if err != nil {
			panic(err)
		}
	}
	os.Exit(benchmark.Run(os.Args[1:], os.Environ(), source, os.Stdout, os.Stderr))
}
