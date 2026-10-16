package middleware

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	unknownRoute = "unknown"
	otherMethod  = "OTHER"
)

// excludedMetricsPaths are not instrumented. Probes and the scrape endpoint are
// noise. The SSE streams stay open for minutes, so they would land entirely in
// the +Inf bucket and pin http_requests_in_flight for their whole lifetime.
var excludedMetricsPaths = map[string]bool{
	"/healthz":                 true,
	"/readyz":                  true,
	"/health":                  true,
	"/health.json":             true,
	"/metrics":                 true,
	"/api/v1/reads/stream":     true,
	"/api/v1/mustering/stream": true,
}

// knownMethods bounds the method label: the method is client-controlled, so
// anything else is reported as OTHER rather than minting a series per value.
var knownMethods = map[string]bool{
	http.MethodGet: true, http.MethodHead: true, http.MethodPost: true,
	http.MethodPut: true, http.MethodPatch: true, http.MethodDelete: true,
	http.MethodConnect: true, http.MethodOptions: true, http.MethodTrace: true,
}

// HTTPMetrics instruments completed application requests. It deliberately uses
// chi's matched route pattern rather than the request path so metric labels do
// not contain resource identifiers.
type HTTPMetrics struct {
	duration *prometheus.HistogramVec
	requests *prometheus.CounterVec
	inFlight prometheus.Gauge
}

// NewHTTPMetrics registers the HTTP collectors with registerer. Reusing an
// already registered collector makes router construction safe when production
// code uses the default registry more than once; callers can pass a dedicated
// registry to isolate tests.
func NewHTTPMetrics(registerer prometheus.Registerer) *HTTPMetrics {
	return &HTTPMetrics{
		duration: register(registerer, prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "http_request_duration_seconds",
			Help: "HTTP request duration in seconds.",
		}, []string{"method", "route", "status"})),
		requests: register(registerer, prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total completed HTTP requests.",
		}, []string{"method", "route", "status"})),
		inFlight: register(registerer, prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "http_requests_in_flight",
			Help: "Current number of in-flight HTTP requests.",
		})),
	}
}

func register[C prometheus.Collector](registerer prometheus.Registerer, collector C) C {
	if err := registerer.Register(collector); err != nil {
		var already prometheus.AlreadyRegisteredError
		if errors.As(err, &already) {
			return already.ExistingCollector.(C)
		}
		panic(err)
	}
	return collector
}

// Middleware records requests after chi has resolved their route pattern. A
// panicking handler is recorded as a 500 before the panic continues to the
// outer Recovery middleware, so failures are not missing from the error rate.
func (m *HTTPMetrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if excludedMetricsPaths[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}

		m.inFlight.Inc()
		started := time.Now()
		writer := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
		defer func() {
			m.inFlight.Dec()
			status := writer.Status()
			recovered := recover()
			if recovered != nil {
				status = http.StatusInternalServerError
			} else if status == 0 {
				status = http.StatusOK
			}
			labels := []string{metricMethod(r.Method), matchedRoute(r), strconv.Itoa(status)}
			m.duration.WithLabelValues(labels...).Observe(time.Since(started).Seconds())
			m.requests.WithLabelValues(labels...).Inc()
			if recovered != nil {
				panic(recovered)
			}
		}()
		next.ServeHTTP(writer, r)
	})
}

func metricMethod(method string) string {
	if knownMethods[method] {
		return method
	}
	return otherMethod
}

func matchedRoute(r *http.Request) string {
	if routeContext := chi.RouteContext(r.Context()); routeContext != nil {
		if pattern := routeContext.RoutePattern(); pattern != "" {
			return pattern
		}
	}
	return unknownRoute
}
