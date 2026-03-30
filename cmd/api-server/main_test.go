package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"yolo-ave-mujica/internal/config"
	"yolo-ave-mujica/internal/server"
)

func testConfig() config.Config {
	return config.Config{
		HTTPAddr:        "127.0.0.1:0",
		ShutdownTimeout: 100 * time.Millisecond,
	}
}

func newTestModules() server.Modules {
	return server.Modules{}
}

func TestRunStopsAfterContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := run(ctx, testConfig(), newTestModules()); err != nil {
		t.Fatalf("expected canceled startup to shut down cleanly, got %v", err)
	}
}

func TestStartBackgroundLoopInvokesTick(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var calls atomic.Int32
	done := make(chan struct{})

	startBackgroundLoop(ctx, 5*time.Millisecond, func(time.Time) error {
		if calls.Add(1) == 1 {
			close(done)
		}
		return nil
	})

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected background loop to invoke tick")
	}
}
