// The metrics package instruments your code.
//
// Set DEBUG=metrics environment variable to print metrics to stdout.
package metrics

import (
	"log"
	"os"
	"strconv"
	"time"

	godebug "github.com/kevinburke/go-debug"
	librato "github.com/mihasya/go-metrics-librato"
	metrics "github.com/rcrowley/go-metrics"
)

const Version = "1.1"

var debug = godebug.Debug("metrics")

// Namespace is the namespace under which all metrics will get incremented.
// Typically this should match up with the running service ("api", "admin",
// "jobs", "parcels", &c).
var Namespace string

func getWithNamespace(metricName string) string {
	if Namespace == "" {
		return metricName
	} else {
		return Namespace + "." + metricName
	}
}

// Start initializes the metrics client. You must call this before sending
// metrics, or metrics will not get published to Librato.
func Start(source string, email string) {
	token, ok := os.LookupEnv("LIBRATO_TOKEN")
	if !ok || token == "" {
		log.Printf("Could not find LIBRATO_TOKEN environment variable; no metrics will be logged")
	} else {
		go librato.Librato(
			metrics.DefaultRegistry,
			15*time.Second,
			email,
			token,
			source,
			[]float64{0.5, 0.99, 1},
			time.Millisecond,
		)
	}
}

// Increment a counter with the given name.
func Increment(name string) {
	mn := getWithNamespace(name)
	c := metrics.GetOrRegisterCounter(mn, nil)
	c.Inc(1)
	debug("increment " + name + " 1")
}

// Measure that the given metric has the given value.
func Measure(name string, value int64) {
	mn := getWithNamespace(name)
	g := metrics.GetOrRegisterGauge(mn, nil)
	g.Update(value)
	debug("measure " + name + " " + strconv.FormatInt(value, 10))
}

// Add a new timing measurement for the given metric.
func Time(name string, value time.Duration) {
	mn := getWithNamespace(name)
	t := metrics.GetOrRegisterTimer(mn, nil)
	t.Update(value)
	debug("time " + name + " " + value.String())
}
