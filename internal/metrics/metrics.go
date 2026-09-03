// Package metrics defines and registers all Prometheus metrics for the gateway.
package metrics

import (
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	HTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_http_requests_total",
			Help: "Total number of HTTP requests by method, path, and status code.",
		},
		[]string{"method", "path", "status"},
	)

	HTTPRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gateway_http_request_duration_seconds",
			Help:    "Histogram of HTTP request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	HTTPInFlightRequests = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "gateway_http_in_flight_requests",
			Help: "Number of HTTP requests currently being processed.",
		},
	)

	ProviderRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_provider_requests_total",
			Help: "Total number of requests to LLM providers.",
		},
		[]string{"provider", "method", "status"},
	)

	ProviderRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gateway_provider_request_duration_seconds",
			Help:    "Histogram of provider request duration in seconds.",
			Buckets: []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60},
		},
		[]string{"provider", "method"},
	)

	ProviderTokensTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_provider_tokens_total",
			Help: "Total token usage by provider and token type (prompt/completion).",
		},
		[]string{"provider", "type"},
	)
)

var initOnce sync.Once

func Init() {
	initOnce.Do(func() {
		prometheus.MustRegister(
			HTTPRequestsTotal,
			HTTPRequestDuration,
			HTTPInFlightRequests,
			ProviderRequestsTotal,
			ProviderRequestDuration,
			ProviderTokensTotal,
		)
	})
}

// Handler returns the Prometheus HTTP handler for the /metrics endpoint.
func Handler() http.Handler {
	return promhttp.Handler()
}
