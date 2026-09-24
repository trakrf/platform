package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
)

const unknownRoute = "unknown"

// HTTPMetrics instruments completed application requests. It deliberately uses
// chi's matched route pattern rather than the request path so metric labels do
// not contain resource identifiers.
type HTTPMetrics struct {
	duration prometheus.ObserverVec
	requests *prometheus.CounterVec
	inFlight prometheus.Gauge
}

// NewHTTPMetrics registers the HTTP collectors with registerer. Reusing an
// already registered collector makes router construction safe when production
// code uses the default registry more than once; callers can pass a dedicated
// registry to isolate tests.
func NewHTTPMetrics(registerer prometheus.Registerer) *HTTPMetrics {
	duration := registerHistogram(registerer, prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "http_request_duration_seconds",
		Help: "HTTP request duration in seconds.",
	}, []string{"method", "route", "status"}))
	requests := registerCounter(registerer, prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total completed HTTP requests.",
	}, []string{"method", "route", "status"}))
	inFlight := registerGauge(registerer, prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "http_requests_in_flight",
		Help: "Current number of in-flight HTTP requests.",
	}))

	return &HTTPMetrics{duration: duration, requests: requests, inFlight: inFlight}
}

func registerHistogram(registerer prometheus.Registerer, collector *prometheus.HistogramVec) *prometheus.HistogramVec {
	if err := registerer.Register(collector); err != nil {
		if existing, ok := err.(prometheus.AlreadyRegisteredError); ok {
			return existing.ExistingCollector.(*prometheus.HistogramVec)
		}
		panic(err)
	}
	return collector
}

func registerCounter(registerer prometheus.Registerer, collector *prometheus.CounterVec) *prometheus.CounterVec {
	if err := registerer.Register(collector); err != nil {
		if existing, ok := err.(prometheus.AlreadyRegisteredError); ok {
			return existing.ExistingCollector.(*prometheus.CounterVec)
		}
		panic(err)
	}
	return collector
}

func registerGauge(registerer prometheus.Registerer, collector prometheus.Gauge) prometheus.Gauge {
	if err := registerer.Register(collector); err != nil {
		if existing, ok := err.(prometheus.AlreadyRegisteredError); ok {
			return existing.ExistingCollector.(prometheus.Gauge)
		}
		panic(err)
	}
	return collector
}

// Middleware records requests after chi has resolved their route pattern.
func (m *HTTPMetrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if excludedMetricsPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		m.inFlight.Inc()
		defer m.inFlight.Dec()

		started := time.Now()
		writer := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(writer, r)

		status := writer.Status()
		if status == 0 {
			status = http.StatusOK
		}
		labels := []string{r.Method, matchedRoute(r), strconv.Itoa(status)}
		m.duration.WithLabelValues(labels...).Observe(time.Since(started).Seconds())
		m.requests.WithLabelValues(labels...).Inc()
	})
}

func excludedMetricsPath(path string) bool {
	return path == "/healthz" || path == "/readyz" || path == "/metrics"
}

func matchedRoute(r *http.Request) string {
	if routeContext := chi.RouteContext(r.Context()); routeContext != nil {
		if pattern := routeContext.RoutePattern(); pattern != "" {
			return pattern
		}
	}
	return unknownRoute
}
