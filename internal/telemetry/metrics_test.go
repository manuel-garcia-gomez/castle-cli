package telemetry

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helper: gather all metric families from the registry and index by name.
func gatherFamilies(t *testing.T, reg *prometheus.Registry) map[string]*dto.MetricFamily {
	t.Helper()
	mfs, err := reg.Gather()
	require.NoError(t, err, "registry.Gather() must not error")

	out := make(map[string]*dto.MetricFamily, len(mfs))
	for _, mf := range mfs {
		out[mf.GetName()] = mf
	}
	return out
}

// helper: find a single metric in a family that matches all supplied label pairs.
func findMetric(mf *dto.MetricFamily, labels map[string]string) *dto.Metric {
	for _, m := range mf.GetMetric() {
		if labelsMatch(m.GetLabel(), labels) {
			return m
		}
	}
	return nil
}

func labelsMatch(pairs []*dto.LabelPair, want map[string]string) bool {
	got := make(map[string]string, len(pairs))
	for _, p := range pairs {
		got[p.GetName()] = p.GetValue()
	}
	for k, v := range want {
		if got[k] != v {
			return false
		}
	}
	return true
}

// ── New() ─────────────────────────────────────────────────────────────────────

func TestNew_ReturnsMetricsWithoutError(t *testing.T) {
	m, err := New()
	require.NoError(t, err)
	require.NotNil(t, m)
}

func TestNew_RegistryIsNotNil(t *testing.T) {
	m, err := New()
	require.NoError(t, err)
	assert.NotNil(t, m.Registry())
}

func TestNew_ThreeMetricFamiliesRegistered(t *testing.T) {
	// Prometheus omits empty vector series from Gather(), so we verify
	// registration via Describe() — which always fires for every declared
	// metric regardless of whether any label-combination has been observed.
	m, err := New()
	require.NoError(t, err)

	want := map[string]bool{
		"castle_command_total":             false,
		"castle_command_duration_seconds":  false,
		"castle_scans_processed_total":     false,
	}

	for _, col := range m.Collectors() {
		ch := make(chan *prometheus.Desc, 10)
		col.Describe(ch)
		close(ch)
		for desc := range ch {
			// desc.String() contains fqName= in the output; extract the name.
			s := desc.String()
			for name := range want {
				if contains(s, `fqName: "`+name+`"`) || contains(s, name) {
					want[name] = true
				}
			}
		}
	}

	for name, found := range want {
		assert.True(t, found, "metric %q must be registered (found via Describe)", name)
	}
}

// contains reports whether substr is present in s.
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

func TestNew_IndependentRegistries_NoCollision(t *testing.T) {
	// Each New() call must produce its own isolated registry without panic.
	m1, err1 := New()
	m2, err2 := New()

	require.NoError(t, err1)
	require.NoError(t, err2)
	assert.NotSame(t, m1.Registry(), m2.Registry(),
		"each Metrics instance must own a separate registry")
}

// ── RecordCommand ──────────────────────────────────────────────────────────────

func TestRecordCommand_IncrementsCommandTotal_Success(t *testing.T) {
	m, err := New()
	require.NoError(t, err)

	m.RecordCommand("scan", "success", 500*time.Millisecond)

	families := gatherFamilies(t, m.Registry())
	mf, ok := families["castle_command_total"]
	require.True(t, ok, "castle_command_total must be present after RecordCommand")

	metric := findMetric(mf, map[string]string{"command": "scan", "status": "success"})
	require.NotNil(t, metric, "metric with labels command=scan,status=success must exist")
	assert.Equal(t, float64(1), metric.GetCounter().GetValue())
}

func TestRecordCommand_IncrementsCommandTotal_Error(t *testing.T) {
	m, err := New()
	require.NoError(t, err)

	m.RecordCommand("deploy", "error", 100*time.Millisecond)

	families := gatherFamilies(t, m.Registry())
	metric := findMetric(families["castle_command_total"],
		map[string]string{"command": "deploy", "status": "error"})
	require.NotNil(t, metric)
	assert.Equal(t, float64(1), metric.GetCounter().GetValue())
}

func TestRecordCommand_AccumulatesMultipleCalls(t *testing.T) {
	m, err := New()
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		m.RecordCommand("init", "success", 10*time.Millisecond)
	}

	families := gatherFamilies(t, m.Registry())
	metric := findMetric(families["castle_command_total"],
		map[string]string{"command": "init", "status": "success"})
	require.NotNil(t, metric)
	assert.Equal(t, float64(5), metric.GetCounter().GetValue())
}

func TestRecordCommand_DifferentCommandsAreSeparate(t *testing.T) {
	m, err := New()
	require.NoError(t, err)

	m.RecordCommand("scan", "success", 1*time.Second)
	m.RecordCommand("deploy", "success", 2*time.Second)
	m.RecordCommand("init", "error", 50*time.Millisecond)

	families := gatherFamilies(t, m.Registry())
	ct := families["castle_command_total"]

	scan := findMetric(ct, map[string]string{"command": "scan", "status": "success"})
	deploy := findMetric(ct, map[string]string{"command": "deploy", "status": "success"})
	initErr := findMetric(ct, map[string]string{"command": "init", "status": "error"})

	require.NotNil(t, scan)
	require.NotNil(t, deploy)
	require.NotNil(t, initErr)

	assert.Equal(t, float64(1), scan.GetCounter().GetValue())
	assert.Equal(t, float64(1), deploy.GetCounter().GetValue())
	assert.Equal(t, float64(1), initErr.GetCounter().GetValue())
}

func TestRecordCommand_ObservesDuration(t *testing.T) {
	m, err := New()
	require.NoError(t, err)

	m.RecordCommand("scan", "success", 250*time.Millisecond)

	families := gatherFamilies(t, m.Registry())
	mf, ok := families["castle_command_duration_seconds"]
	require.True(t, ok, "castle_command_duration_seconds must exist")

	metric := findMetric(mf, map[string]string{"command": "scan"})
	require.NotNil(t, metric, "histogram for command=scan must exist")

	h := metric.GetHistogram()
	assert.Equal(t, uint64(1), h.GetSampleCount(), "one observation expected")
	assert.InDelta(t, 0.25, h.GetSampleSum(), 0.01, "sum should be ~0.25s")
}

func TestRecordCommand_DurationHistogramAccumulates(t *testing.T) {
	m, err := New()
	require.NoError(t, err)

	m.RecordCommand("deploy", "success", 100*time.Millisecond)
	m.RecordCommand("deploy", "error", 200*time.Millisecond)

	families := gatherFamilies(t, m.Registry())
	metric := findMetric(families["castle_command_duration_seconds"],
		map[string]string{"command": "deploy"})
	require.NotNil(t, metric)

	h := metric.GetHistogram()
	assert.Equal(t, uint64(2), h.GetSampleCount())
	assert.InDelta(t, 0.3, h.GetSampleSum(), 0.01)
}

// ── RecordScanFinding ──────────────────────────────────────────────────────────

func TestRecordScanFinding_IncrementsBySeverity(t *testing.T) {
	m, err := New()
	require.NoError(t, err)

	m.RecordScanFinding("critical", 3)
	m.RecordScanFinding("high", 7)

	families := gatherFamilies(t, m.Registry())
	mf, ok := families["castle_scans_processed_total"]
	require.True(t, ok, "castle_scans_processed_total must be present")

	critical := findMetric(mf, map[string]string{"severity": "critical"})
	high := findMetric(mf, map[string]string{"severity": "high"})

	require.NotNil(t, critical)
	require.NotNil(t, high)
	assert.Equal(t, float64(3), critical.GetCounter().GetValue())
	assert.Equal(t, float64(7), high.GetCounter().GetValue())
}

func TestRecordScanFinding_Accumulates(t *testing.T) {
	m, err := New()
	require.NoError(t, err)

	m.RecordScanFinding("medium", 2)
	m.RecordScanFinding("medium", 5)

	families := gatherFamilies(t, m.Registry())
	medium := findMetric(families["castle_scans_processed_total"],
		map[string]string{"severity": "medium"})
	require.NotNil(t, medium)
	assert.Equal(t, float64(7), medium.GetCounter().GetValue())
}

func TestRecordScanFinding_ZeroCountIsNoop(t *testing.T) {
	m, err := New()
	require.NoError(t, err)

	// Zero count must not create a metric series.
	m.RecordScanFinding("low", 0)

	families := gatherFamilies(t, m.Registry())
	mf := families["castle_scans_processed_total"]
	// mf may be nil (no observations yet) or present but without "low".
	if mf != nil {
		metric := findMetric(mf, map[string]string{"severity": "low"})
		assert.Nil(t, metric, "zero-count call must not create a counter series")
	}
}

func TestRecordScanFinding_NegativeCountIsNoop(t *testing.T) {
	m, err := New()
	require.NoError(t, err)

	m.RecordScanFinding("info", -1)

	families := gatherFamilies(t, m.Registry())
	mf := families["castle_scans_processed_total"]
	if mf != nil {
		metric := findMetric(mf, map[string]string{"severity": "info"})
		assert.Nil(t, metric, "negative-count call must not create a counter series")
	}
}

func TestRecordScanFinding_AllSeverities(t *testing.T) {
	m, err := New()
	require.NoError(t, err)

	severities := map[string]int{
		"critical": 1,
		"high":     2,
		"medium":   3,
		"low":      4,
		"info":     5,
	}
	for sev, count := range severities {
		m.RecordScanFinding(sev, count)
	}

	families := gatherFamilies(t, m.Registry())
	mf := families["castle_scans_processed_total"]
	require.NotNil(t, mf)

	for sev, want := range severities {
		metric := findMetric(mf, map[string]string{"severity": sev})
		require.NotNil(t, metric, "metric for severity=%s must exist", sev)
		assert.Equal(t, float64(want), metric.GetCounter().GetValue(),
			"severity=%s count mismatch", sev)
	}
}

// ── Concurrency safety ─────────────────────────────────────────────────────────

func TestMetrics_ConcurrentAccess_NoRace(t *testing.T) {
	// Run with -race to validate; this test is structurally correct regardless.
	m, err := New()
	require.NoError(t, err)

	done := make(chan struct{})
	const goroutines = 20

	for i := 0; i < goroutines; i++ {
		go func() {
			m.RecordCommand("scan", "success", 10*time.Millisecond)
			m.RecordScanFinding("high", 1)
			done <- struct{}{}
		}()
	}
	for i := 0; i < goroutines; i++ {
		<-done
	}

	families := gatherFamilies(t, m.Registry())

	ct := findMetric(families["castle_command_total"],
		map[string]string{"command": "scan", "status": "success"})
	require.NotNil(t, ct)
	assert.Equal(t, float64(goroutines), ct.GetCounter().GetValue())

	sf := findMetric(families["castle_scans_processed_total"],
		map[string]string{"severity": "high"})
	require.NotNil(t, sf)
	assert.Equal(t, float64(goroutines), sf.GetCounter().GetValue())
}
