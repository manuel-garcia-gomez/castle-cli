// Package telemetry provides Prometheus-based SRE metrics for castle-cli.
//
// Three metric families are exposed:
//
//   - castle_command_total        – CounterVec (labels: command, status)
//   - castle_command_duration_seconds – HistogramVec (label: command)
//   - castle_scans_processed_total    – CounterVec (label: severity)
//
// All metrics are registered on a dedicated prometheus.Registry so the package
// can be exercised in tests without touching the global default registry and
// without causing duplicate-registration panics across test runs.
package telemetry

import (
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics holds the three SRE metric families for castle-cli.
// Create one with New and keep it for the lifetime of the process.
type Metrics struct {
	registry *prometheus.Registry

	// commandTotal counts every completed command execution.
	// Labels: command (e.g. "scan", "deploy", "init"), status ("success"|"error").
	commandTotal *prometheus.CounterVec

	// commandDuration records the wall-clock duration of each command in seconds.
	// Label: command (same values as commandTotal).
	commandDuration *prometheus.HistogramVec

	// scansProcessed counts security scan findings by severity bucket.
	// Label: severity (e.g. "critical", "high", "medium", "low", "info").
	scansProcessed *prometheus.CounterVec
}

// New creates and registers all castle-cli metrics on a fresh, isolated
// prometheus.Registry. It never panics; any registration error is returned
// so the caller can decide how to proceed (log-and-continue is fine for a CLI).
func New() (*Metrics, error) {
	reg := prometheus.NewRegistry()

	commandTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "castle_command_total",
			Help: "Total number of castle CLI command executions, partitioned by command name and final status.",
		},
		[]string{"command", "status"},
	)

	commandDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "castle_command_duration_seconds",
			Help: "Wall-clock duration of castle CLI command executions in seconds.",
			// Buckets cover sub-second interactive commands up to a 5-minute scan.
			Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300},
		},
		[]string{"command"},
	)

	scansProcessed := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "castle_scans_processed_total",
			Help: "Total number of security scan findings processed by castle, partitioned by severity.",
		},
		[]string{"severity"},
	)

	for _, c := range []prometheus.Collector{commandTotal, commandDuration, scansProcessed} {
		if err := reg.Register(c); err != nil {
			return nil, err
		}
	}

	slog.Info("telemetry: metrics registered",
		"metrics", []string{
			"castle_command_total",
			"castle_command_duration_seconds",
			"castle_scans_processed_total",
		},
	)

	return &Metrics{
		registry:        reg,
		commandTotal:    commandTotal,
		commandDuration: commandDuration,
		scansProcessed:  scansProcessed,
	}, nil
}

// RecordCommand increments castle_command_total{command, status} and observes
// the elapsed duration in castle_command_duration_seconds{command}.
//
// status must be either "success" or "error".
// elapsed is the wall-clock duration of the command (e.g. time.Since(start)).
func (m *Metrics) RecordCommand(command, status string, elapsed time.Duration) {
	m.commandTotal.WithLabelValues(command, status).Inc()
	m.commandDuration.WithLabelValues(command).Observe(elapsed.Seconds())

	slog.Info("telemetry: command recorded",
		"command", command,
		"status", status,
		"duration_s", elapsed.Seconds(),
	)
}

// RecordScanFinding increments castle_scans_processed_total{severity} by count.
// severity should be one of "critical", "high", "medium", "low", "info".
// count must be ≥ 0; negative values are silently treated as 0.
func (m *Metrics) RecordScanFinding(severity string, count int) {
	if count <= 0 {
		return
	}
	m.scansProcessed.WithLabelValues(severity).Add(float64(count))

	slog.Info("telemetry: scan findings recorded",
		"severity", severity,
		"count", count,
	)
}

// Registry returns the underlying prometheus.Registry so callers can expose
// the metrics over HTTP (e.g. via promhttp.HandlerFor) when needed.
func (m *Metrics) Registry() *prometheus.Registry {
	return m.registry
}

// Collectors returns the three prometheus.Collector instances that back the
// castle-cli metric families. Useful in tests and diagnostics to verify
// registration without relying on Gather() (which skips empty vector series).
func (m *Metrics) Collectors() []prometheus.Collector {
	return []prometheus.Collector{
		m.commandTotal,
		m.commandDuration,
		m.scansProcessed,
	}
}
