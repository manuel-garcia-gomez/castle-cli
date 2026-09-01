package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runScanCmd executes "castle scan" with the given args and captures I/O.
// The scanner binaries (trivy, semgrep) may or may not be present; the
// implementation falls back to a stub report, so all tests here work without
// external tooling installed.
func runScanCmd(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer

	root := newRootCmd()
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SetArgs(args)

	err = root.Execute()
	return outBuf.String(), errBuf.String(), err
}

// ── Happy path — trivy (stub fallback when binary absent) ────────────────────

func TestScan_Trivy_NoError(t *testing.T) {
	_, _, err := runScanCmd(t, "scan", "--type", "trivy", "--target", ".")
	assert.NoError(t, err, "castle scan --type trivy must not return an error")
}

func TestScan_Trivy_OutputContainsScanComplete(t *testing.T) {
	out, _, err := runScanCmd(t, "scan", "--type", "trivy", "--target", ".")
	require.NoError(t, err)
	assert.True(t, strings.Contains(out, "Scan complete"),
		"stdout must contain 'Scan complete'")
}

// ── Happy path — semgrep (stub fallback when binary absent) ──────────────────

func TestScan_Semgrep_NoError(t *testing.T) {
	_, _, err := runScanCmd(t, "scan", "--type", "semgrep", "--target", ".")
	assert.NoError(t, err, "castle scan --type semgrep must not return an error")
}

func TestScan_Semgrep_OutputContainsScanComplete(t *testing.T) {
	out, _, err := runScanCmd(t, "scan", "--type", "semgrep", "--target", ".")
	require.NoError(t, err)
	assert.True(t, strings.Contains(out, "Scan complete"),
		"stdout must contain 'Scan complete'")
}

// ── Invalid scanner type ──────────────────────────────────────────────────────

func TestScan_UnknownType_ReturnsError(t *testing.T) {
	_, _, err := runScanCmd(t, "scan", "--type", "bandit")
	require.Error(t, err, "unsupported scanner type must return an error")
}

// ── --upload without api_key ──────────────────────────────────────────────────

func TestScan_Upload_WithoutAPIKey_ReturnsError(t *testing.T) {
	// cfg is the package-level config; reset after test.
	origKey := cfg.DefectDojo.APIKey
	cfg.DefectDojo.APIKey = ""
	t.Cleanup(func() { cfg.DefectDojo.APIKey = origKey })

	_, _, err := runScanCmd(t, "scan", "--type", "trivy", "--target", ".", "--upload")
	require.Error(t, err, "--upload without api_key must return an error")
	assert.True(t, strings.Contains(err.Error(), "api_key"),
		"error message must mention api_key")
}

// ── No temporary files left on disk after scan ───────────────────────────────

func TestScan_NoTempFileResidueAfterRun(t *testing.T) {
	tmpBefore, err := os.ReadDir(os.TempDir())
	require.NoError(t, err)

	_, _, _ = runScanCmd(t, "scan", "--type", "trivy", "--target", ".")

	tmpAfter, err := os.ReadDir(os.TempDir())
	require.NoError(t, err)

	// Allow for transient files from other processes: just ensure we didn't
	// grow the count beyond a small tolerance.
	assert.LessOrEqual(t, len(tmpAfter), len(tmpBefore)+2,
		"scan must clean up its temporary report file")
}

// ── defectDojoScanType mapping ───────────────────────────────────────────────

func TestDefectDojoScanType_Trivy(t *testing.T) {
	assert.Equal(t, "Trivy Scan", defectDojoScanType("trivy"))
}

func TestDefectDojoScanType_Semgrep(t *testing.T) {
	assert.Equal(t, "Semgrep JSON Report", defectDojoScanType("semgrep"))
}

func TestDefectDojoScanType_Unknown_PassThrough(t *testing.T) {
	assert.Equal(t, "custom-type", defectDojoScanType("custom-type"),
		"unknown scanner type must be passed through unchanged")
}

// ── Panic safety ──────────────────────────────────────────────────────────────

func TestScan_NoPanic_Trivy(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("castle scan panicked: %v", r)
		}
	}()
	_, _, _ = runScanCmd(t, "scan", "--type", "trivy", "--target", ".")
}

func TestScan_NoPanic_Semgrep(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("castle scan panicked: %v", r)
		}
	}()
	_, _, _ = runScanCmd(t, "scan", "--type", "semgrep", "--target", ".")
}
