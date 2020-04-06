package metrics

import (
	"context"
	"fmt"
	"time"

	metrics "github.com/kevinburke/go-simple-metrics"
)

type Metrics interface {
	Increment(metric string) error
	Time(metric string, duration time.Duration) error
	Measure(metric string, value int64) error
	// Run starts the metrics client and blocks until it completes.
	Run(context.Context, interface{}) error
}

type SimpleMetrics struct {
}

func (m SimpleMetrics) Increment(metric string) error {
	metrics.Increment(metric)
	return nil
}

func (m SimpleMetrics) Time(metric string, duration time.Duration) error {
	metrics.Time(metric, duration)
	return nil
}

func (m SimpleMetrics) Measure(metric string, value int64) error {
	metrics.Measure(metric, value)
	return nil
}

func (m SimpleMetrics) Exclude(_ string) bool {
	return false
}

// LibratoConfig is used with the go-simple-metrics (default) client. Pass
// a LibratoConfig instance as the second argument to Run.
type LibratoConfig struct {
	Namespace string
	Source    string
	Email     string
}

func (m SimpleMetrics) Run(ctx context.Context, cfg interface{}) error {
	lcfg, ok := cfg.(LibratoConfig)
	if !ok {
		return fmt.Errorf("cannot cast cfg (%v) to a LibratoConfig", cfg)
	}
	if lcfg.Namespace != "" {
		metrics.Namespace = lcfg.Namespace
	}
	// this spawns a goroutine
	metrics.Start(lcfg.Source, lcfg.Email)
	<-ctx.Done()
	return ctx.Err()
}

func Increment(metric string) error {
	if Exclude(metric) {
		return nil
	}
	return Client.Increment(metric)
}

func Time(metric string, duration time.Duration) error {
	if Exclude(metric) {
		return nil
	}
	return Client.Time(metric, duration)
}

func Measure(metric string, value int64) error {
	if Exclude(metric) {
		return nil
	}
	return Client.Measure(metric, value)
}

func Run(ctx context.Context, cfg interface{}) error {
	return Client.Run(ctx, cfg)
}

var Client Metrics = SimpleMetrics{}

// Exclude reports if a metric should be excluded from being reported. By
// default all metrics are included, though callers can override this function.
var Exclude = func(metric string) bool {
	return false
}
