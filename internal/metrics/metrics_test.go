package metrics_test

import (
	"testing"

	dto "github.com/prometheus/client_model/go"

	"github.com/paul/clock-server/internal/metrics"
)

func TestNewMetricsRegistersAllMetrics(t *testing.T) {
	m := metrics.NewMetrics()
	if m == nil {
		t.Fatal("expected non-nil Metrics")
	}
	if m.RequestsTotal == nil {
		t.Error("expected non-nil RequestsTotal")
	}
	if m.AuthFailuresTotal == nil {
		t.Error("expected non-nil AuthFailuresTotal")
	}
	if m.DispatchDuration == nil {
		t.Error("expected non-nil DispatchDuration")
	}
	if m.DispatchErrorsTotal == nil {
		t.Error("expected non-nil DispatchErrorsTotal")
	}
}

func TestRegistryContainsExpectedMetricNames(t *testing.T) {
	m := metrics.NewMetrics()
	mfs, err := m.Registry().Gather()
	if err != nil {
		// No data has been recorded yet — Gather may return empty but shouldn't error.
		t.Fatalf("unexpected gather error: %v", err)
	}

	// Before any increments, no metric families are returned (lazy registration).
	// Record at least one observation per metric so they appear.
	m.RequestsTotal.WithLabelValues("GET", "/health", "200").Inc()
	m.AuthFailuresTotal.WithLabelValues("missing_token").Inc()
	m.DispatchDuration.WithLabelValues("mqtt").Observe(0.001)
	m.DispatchErrorsTotal.WithLabelValues("rest").Inc()

	mfs, err = m.Registry().Gather()
	if err != nil {
		t.Fatalf("gather error after recording: %v", err)
	}

	want := map[string]bool{
		"clock_requests_total":            false,
		"clock_auth_failures_total":       false,
		"clock_dispatch_duration_seconds": false,
		"clock_dispatch_errors_total":     false,
	}
	for _, mf := range mfs {
		if _, ok := want[mf.GetName()]; ok {
			want[mf.GetName()] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("expected metric %q to be registered but it was not found", name)
		}
	}
}

func TestMetricsIsolation(t *testing.T) {
	// Two separate Metrics instances must not share counters.
	m1 := metrics.NewMetrics()
	m2 := metrics.NewMetrics()

	m1.RequestsTotal.WithLabelValues("GET", "/test", "200").Inc()

	mfs, err := m2.Registry().Gather()
	if err != nil {
		t.Fatalf("gather error: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() == "clock_requests_total" {
			for _, metric := range mf.GetMetric() {
				if getCounterValue(metric) > 0 {
					t.Error("m2 should not share state with m1, but counter is non-zero")
				}
			}
		}
	}
}

func getCounterValue(m *dto.Metric) float64 {
	if m.Counter != nil && m.Counter.Value != nil {
		return *m.Counter.Value
	}
	return 0
}
