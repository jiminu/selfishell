package testutil

import (
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
)

// Each writer's executions fork while the others write, as parallel tests do.
func TestWriteFileSurvivesConcurrentForks(t *testing.T) {
	dir := t.TempDir()
	var wg sync.WaitGroup
	var busy atomic.Int32
	for w := range 8 {
		wg.Go(func() {
			for i := range 50 {
				path := fmt.Sprintf("%s/tool-%d-%d", dir, w, i)
				if err := WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
					t.Error(err)
					return
				}
				if err := exec.Command(path).Run(); errors.Is(err, syscall.ETXTBSY) {
					busy.Add(1)
				} else if err != nil {
					t.Error(err)
					return
				}
			}
		})
	}
	wg.Wait()
	if n := busy.Load(); n != 0 {
		t.Fatalf("%d of 400 executions failed with ETXTBSY", n)
	}
}
