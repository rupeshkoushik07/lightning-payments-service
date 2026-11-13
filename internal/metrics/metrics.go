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

var (
	// Precise money maths: routing fees actually paid, in whole sats.
	FeesPaidSat = promauto.NewCounter(prometheus.CounterOpts{
		Name: "lnpay_fees_paid_sats_total",
		Help: "Routing fees paid on successful payments, in sats.",
	})
	// Share of the fee limit each payment used. Above 1.0 would be a bug.
	FeeLimitUsed = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lnpay_fee_limit_used_ratio",
		Help:    "Fee paid divided by the fee limit, per successful payment.",
		Buckets: []float64{0, 0.1, 0.25, 0.5, 0.75, 0.9, 1, 1.5},
	})

	// Partial failures: one payment stuck 2 hours is worse than five stuck 2 seconds.
	OldestInFlight = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lnpay_oldest_in_flight_seconds",
		Help: "Age of the oldest IN_FLIGHT payment at the last tracker round (0 if none).",
	})

	// Liquidity: what bob can still send. Low means payments will start failing.
	SendableSat = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lnpay_channel_sendable_sats",
		Help: "Local balance of bob's active channels.",
	})
	ActiveChannels = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lnpay_channels_active",
		Help: "Bob's channels whose partner is connected.",
	})

	// Reconciliation, read from the database (survives restarts, unlike counters).
	BooksSentSat = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lnpay_books_sent_sats", Help: "Sum of SUCCEEDED payments.",
	})
	BooksReceivedSat = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lnpay_books_received_sats", Help: "Sum of PAID invoices.",
	})
	BooksMismatch = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "lnpay_books_mismatches",
		Help: "Rows that do not reconcile. not_credited and amount_differs must be 0.",
	}, []string{"kind"}) // not_credited | amount_differs | paid_outside
)
