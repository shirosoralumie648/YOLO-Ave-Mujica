package artifacts

import (
	"context"
	"sync"
)

type BuildRunner struct {
	queue       chan int64
	run         func(context.Context, int64) error
	concurrency int
	startOnce   sync.Once
}

func NewBuildRunner(concurrency int, run func(context.Context, int64) error) *BuildRunner {
	if concurrency <= 0 {
		concurrency = 1
	}
	return &BuildRunner{
		queue:       make(chan int64, 128),
		run:         run,
		concurrency: concurrency,
	}
}

func (r *BuildRunner) Start(ctx context.Context) {
	if r == nil || r.run == nil {
		return
	}
	r.startOnce.Do(func() {
		for i := 0; i < r.concurrency; i++ {
			go func() {
				for {
					select {
					case <-ctx.Done():
						return
					case artifactID := <-r.queue:
						_ = r.run(ctx, artifactID)
					}
				}
			}()
		}
	})
}

func (r *BuildRunner) Enqueue(artifactID int64) {
	if r == nil {
		return
	}
	r.queue <- artifactID
}
