package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	clientmodel "github.com/prometheus/client_model/go"
)

func TestHTTPMetricsRecordsMatchedRouteAndStatus(t *testing.T) {
	registry := prometheus.NewRegistry()
	router := metricsTestRouter(NewHTTPMetrics(registry), http.StatusTeapot, nil)
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/orgs/201599710/invitations", nil))

	labels := map[string]string{"method": http.MethodGet, "route": "/api/v1/orgs/{orgID}/invitations", "status": strconv.Itoa(http.StatusTeapot)}
	assertMetric(t, registry, "http_requests_total", labels, func(metric *clientmodel.Metric) bool {
		return metric.GetCounter().GetValue() == 1
	})
	assertMetric(t, registry, "http_request_duration_seconds", labels, func(metric *clientmodel.Metric) bool {
		return metric.GetHistogram().GetSampleCount() == 1 && len(metric.GetHistogram().GetBucket()) > 0
	})
}

func TestHTTPMetricsInFlightReturnsToZero(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewHTTPMetrics(registry)
	started := make(chan struct{})
	release := make(chan struct{})
	router := metricsTestRouter(metrics, http.StatusOK, func() {
		close(started)
		<-release
	})

	var wait sync.WaitGroup
	wait.Add(1)
	go func() {
		defer wait.Done()
		router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/orgs/1/invitations", nil))
	}()
	<-started
	assertMetric(t, registry, "http_requests_in_flight", nil, func(metric *clientmodel.Metric) bool {
		return metric.GetGauge().GetValue() == 1
	})
	close(release)
	wait.Wait()
	assertMetric(t, registry, "http_requests_in_flight", nil, func(metric *clientmodel.Metric) bool {
		return metric.GetGauge().GetValue() == 0
	})
}

func TestHTTPMetricsExcludesProbeAndMetricRoutes(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewHTTPMetrics(registry)
	router := chi.NewRouter()
	router.Use(metrics.Middleware)
	for _, path := range []string{"/healthz", "/readyz"} {
		router.Get(path, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
		router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}
	router.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	metricsResponse := httptest.NewRecorder()
	router.ServeHTTP(metricsResponse, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if metricsResponse.Code != http.StatusOK {
		t.Fatalf("/metrics status = %d, want %d", metricsResponse.Code, http.StatusOK)
	}

	assertMetricAbsent(t, registry, "http_requests_total")
	assertMetricAbsent(t, registry, "http_request_duration_seconds")
}

func TestHTTPMetricsRegistrationCanBeRepeated(t *testing.T) {
	registry := prometheus.NewRegistry()
	first := NewHTTPMetrics(registry)
	second := NewHTTPMetrics(registry)
	metricsTestRouter(first, http.StatusOK, nil)
	metricsTestRouter(second, http.StatusOK, nil)
}

func metricsTestRouter(metrics *HTTPMetrics, status int, beforeResponse func()) *chi.Mux {
	router := chi.NewRouter()
	router.Use(metrics.Middleware)
	router.Get("/api/v1/orgs/{orgID}/invitations", func(w http.ResponseWriter, _ *http.Request) {
		if beforeResponse != nil {
			beforeResponse()
		}
		w.WriteHeader(status)
	})
	return router
}

func assertMetric(t *testing.T, registry *prometheus.Registry, name string, labels map[string]string, matches func(*clientmodel.Metric) bool) {
	t.Helper()
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			if labelsMatch(metric.GetLabel(), labels) && matches(metric) {
				return
			}
		}
	}
	t.Fatalf("%s with labels %v was not found", name, labels)
}

func assertMetricAbsent(t *testing.T, registry *prometheus.Registry, name string) {
	t.Helper()
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() == name && len(family.GetMetric()) != 0 {
			t.Fatalf("%s unexpectedly contains samples", name)
		}
	}
}

func labelsMatch(actual []*clientmodel.LabelPair, expected map[string]string) bool {
	if len(actual) != len(expected) {
		return false
	}
	for _, label := range actual {
		if expected[label.GetName()] != label.GetValue() {
			return false
		}
	}
	return true
}
