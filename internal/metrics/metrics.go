// Package metrics holds the numbers Prometheus reads from GET /metrics.
// Grafana draws them. Each one answers one question an operator asks at 2 a.m.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Are payments working? One count per final answer, by who settled it.
	PaymentsFinished = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "lnpay_payments_finished_total",
		Help: "Payments that reached a final status.",
	}, []string{"status", "by"}) // status: SUCCEEDED|FAILED, by: sender|tracker

	// Payments the sender could not settle and left IN_FLIGHT for the tracker.
	PaymentsUnknown = promauto.NewCounter(prometheus.CounterOpts{
		Name: "lnpay_payments_unknown_total",
		Help: "Pay calls that ended without a final answer (left IN_FLIGHT).",
	})

	// How many payments are stuck right now, as seen by the last tracker round.
	InFlight = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lnpay_payments_in_flight",
		Help: "Payments IN_FLIGHT at the last tracker round.",
	})

	// How slow is the node to answer a Pay call?
	PayDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lnpay_pay_duration_seconds",
		Help:    "Time for the node's Pay call to return.",
		Buckets: []float64{0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60},
	})

	// Is the node reachable? result: ok|error.
	NodeCalls = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "lnpay_node_calls_total",
		Help: "Calls to the paying LND node.",
	}, []string{"call", "result"}) // call: pay|track

	// Double-pay attempts the database blocked (same invoice twice).
	DuplicatesBlocked = promauto.NewCounter(prometheus.CounterOpts{
		Name: "lnpay_duplicates_blocked_total",
		Help: "Pay requests refused because the invoice was already being paid or paid.",
	})
)

// result turns an error into a label.
func Result(err error) string {
	if err != nil {
		return "error"
	}
	return "ok"
}
