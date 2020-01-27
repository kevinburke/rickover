// Tests for the jobs dequeuer.
package dequeuer

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	log "github.com/inconshreveable/log15"
	"github.com/kevinburke/rickover/newmodels"
)

type DummyProcessor struct {
	Count int64
}

func (dp *DummyProcessor) DoWork(context.Context, *newmodels.QueuedJob) error {
	atomic.AddInt64(&dp.Count, 1)
	return nil
}

type channelProcessor struct {
	log.Logger
	Count int64
	Ch    chan struct{}
}

func (dp *channelProcessor) DoWork(ctx context.Context, qj *newmodels.QueuedJob) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case dp.Ch <- struct{}{}:
		atomic.AddInt64(&dp.Count, 1)
		return nil
	case <-time.After(100 * time.Millisecond):
		return errors.New("channel send timed out")
	}
}
func (dp *channelProcessor) Sleep(_ int32) time.Duration {
	return 0
}
