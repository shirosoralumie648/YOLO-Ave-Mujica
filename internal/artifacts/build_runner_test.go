package artifacts

import (
	"context"
	"testing"
	"time"
)

func TestBuildRunnerLimitsConcurrentBuilds(t *testing.T) {
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	runner := NewBuildRunner(1, func(context.Context, int64) error {
		started <- struct{}{}
		<-release
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.Start(ctx)
	runner.Enqueue(1)
	runner.Enqueue(2)

	<-started
	select {
	case <-started:
		t.Fatal("expected only one build to start while concurrency=1")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	<-started
}
