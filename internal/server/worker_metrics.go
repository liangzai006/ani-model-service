package server

import "github.com/prometheus/client_golang/prometheus"

type WorkerMetrics struct {
	backlog, oldest                                   prometheus.Gauge
	claims, retries, checksumFailures, providerErrors prometheus.Counter
}

func NewWorkerMetrics(reg prometheus.Registerer) *WorkerMetrics {
	m := &WorkerMetrics{
		backlog:          prometheus.NewGauge(prometheus.GaugeOpts{Name: "model_worker_backlog", Help: "Number of due import tasks waiting for work."}),
		oldest:           prometheus.NewGauge(prometheus.GaugeOpts{Name: "model_worker_oldest_age_seconds", Help: "Age in seconds of the oldest due import task."}),
		claims:           prometheus.NewCounter(prometheus.CounterOpts{Name: "model_worker_claims_total", Help: "Import task lease claims."}),
		retries:          prometheus.NewCounter(prometheus.CounterOpts{Name: "model_worker_retries_total", Help: "Import task retries."}),
		checksumFailures: prometheus.NewCounter(prometheus.CounterOpts{Name: "model_worker_checksum_failures_total", Help: "Checksum verification failures."}),
		providerErrors:   prometheus.NewCounter(prometheus.CounterOpts{Name: "model_worker_provider_errors_total", Help: "External provider errors."}),
	}
	reg.MustRegister(m.backlog, m.oldest, m.claims, m.retries, m.checksumFailures, m.providerErrors)
	return m
}
func (m *WorkerMetrics) SetBacklog(count int, oldestAgeSeconds float64) {
	m.backlog.Set(float64(count))
	m.oldest.Set(oldestAgeSeconds)
}
func (m *WorkerMetrics) Claim()           { m.claims.Inc() }
func (m *WorkerMetrics) Retry()           { m.retries.Inc() }
func (m *WorkerMetrics) ChecksumFailure() { m.checksumFailures.Inc() }
func (m *WorkerMetrics) ProviderError()   { m.providerErrors.Inc() }
