// Package metrics exposes Prometheus metrics for dined on a separate internal
// listener, never on the public router. It owns its registry instead of using
// the global default, so only the collectors registered here are exported.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Result label values shared by the domain counters.
const (
	ResultSuccess = "success"
	ResultFailure = "failure"
)

// JobSessionCleanup is the job label for the expired-session cleanup.
const JobSessionCleanup = "session_cleanup"

// ProviderGoogle is the provider label for Google sign-in.
const ProviderGoogle = "google"

var (
	loginFailures = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "dined_login_failures_total",
		Help: "Sign-ins refused after Google authenticated the account, because the email is unverified or not on the allowlist.",
	})
	oauthFailures = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "dined_oauth_failures_total",
		Help: "OAuth callbacks that failed before a session was created, such as a bad state, a provider error or a failed code exchange.",
	}, []string{"provider"})
	uploads = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "dined_uploads_total",
		Help: "Dine photos submitted with a create or update, by whether the save stored them.",
	}, []string{"result"})
	jobs = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "dined_jobs_total",
		Help: "Background job runs by job and result.",
	}, []string{"job", "result"})
)

func init() {
	// Start every known series at zero so rate() and alerts see it before the
	// first event.
	loginFailures.Add(0)
	oauthFailures.WithLabelValues(ProviderGoogle)
	for _, result := range []string{ResultSuccess, ResultFailure} {
		uploads.WithLabelValues(result)
		jobs.WithLabelValues(JobSessionCleanup, result)
	}
}

// NewRegistry returns a registry holding the Go runtime and process
// collectors, the HTTP RED metrics, the domain counters and any extra
// collectors, such as the database pool collector.
func NewRegistry(extra ...prometheus.Collector) *prometheus.Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		httpRequests,
		httpDuration,
		loginFailures,
		oauthFailures,
		uploads,
		jobs,
	)
	reg.MustRegister(extra...)
	return reg
}

// Handler serves GET /metrics from reg. Every other path returns 404.
func Handler(reg *prometheus.Registry) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))
	return mux
}

// LoginFailure counts a sign-in refused for an authenticated Google account.
func LoginFailure() {
	loginFailures.Inc()
}

// OAuthFailure counts a failed OAuth callback for provider.
func OAuthFailure(provider string) {
	oauthFailures.WithLabelValues(provider).Inc()
}

// Uploads counts n submitted photos with result ResultSuccess or ResultFailure.
func Uploads(n int, result string) {
	if n > 0 {
		uploads.WithLabelValues(result).Add(float64(n))
	}
}

// Job counts one run of job with result ResultSuccess or ResultFailure.
func Job(job, result string) {
	jobs.WithLabelValues(job, result).Inc()
}
