package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics holds all Prometheus metrics for the clock-server.
// Use NewMetrics to construct with an explicit registry.
type Metrics struct {
	registry *prometheus.Registry

	RequestsTotal       *prometheus.CounterVec
	AuthFailuresTotal   *prometheus.CounterVec
	DispatchDuration    *prometheus.HistogramVec
	DispatchErrorsTotal *prometheus.CounterVec
}

// NewMetrics creates and registers all metrics with an explicit (non-global) registry.
func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()

	requestsTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "clock_requests_total",
		Help: "Total number of HTTP requests handled, by method, path, and status.",
	}, []string{"method", "path", "status"})

	authFailuresTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "clock_auth_failures_total",
		Help: "Total number of authentication failures, by reason.",
	}, []string{"reason"})

	dispatchDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "clock_dispatch_duration_seconds",
		Help:    "Duration of command dispatch operations, by adapter.",
		Buckets: prometheus.DefBuckets,
	}, []string{"adapter"})

	dispatchErrorsTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "clock_dispatch_errors_total",
		Help: "Total number of command dispatch errors, by adapter.",
	}, []string{"adapter"})

	reg.MustRegister(requestsTotal, authFailuresTotal, dispatchDuration, dispatchErrorsTotal)

	return &Metrics{
		registry:            reg,
		RequestsTotal:       requestsTotal,
		AuthFailuresTotal:   authFailuresTotal,
		DispatchDuration:    dispatchDuration,
		DispatchErrorsTotal: dispatchErrorsTotal,
	}
}

// Registry returns the underlying Prometheus registry for use with promhttp.HandlerFor.
func (m *Metrics) Registry() *prometheus.Registry {
	return m.registry
}
