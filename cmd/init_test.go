package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runInitInDir executes "castle init" with the given args inside dir,
// which becomes the working directory for the duration of the test.
// It restores the original working directory via t.Cleanup.
func runInitInDir(t *testing.T, dir string, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	orig, wdErr := os.Getwd()
	require.NoError(t, wdErr, "failed to get current working directory")
	require.NoError(t, os.Chdir(dir), "failed to chdir to temp dir")
	t.Cleanup(func() { _ = os.Chdir(orig) })

	var outBuf, errBuf bytes.Buffer
	root := newRootCmd()
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SetArgs(args)

	err = root.Execute()
	return outBuf.String(), errBuf.String(), err
}

// ── Happy path ────────────────────────────────────────────────────────────────

func TestInit_CreatesThreeManifests(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runInitInDir(t, dir, "init", "--app", "test-app", "--team", "platform")
	require.NoError(t, err)

	infra := filepath.Join(dir, "infra", "test-app")
	for _, name := range []string{"namespace.yaml", "peer-authentication.yaml", "gatekeeper.yaml"} {
		path := filepath.Join(infra, name)
		info, statErr := os.Stat(path)
		require.NoError(t, statErr, "manifest %q must exist", name)
		assert.Greater(t, info.Size(), int64(0), "manifest %q must not be empty", name)
	}
}

func TestInit_OutputMentionsApp(t *testing.T) {
	dir := t.TempDir()
	out, _, err := runInitInDir(t, dir, "init", "--app", "my-svc", "--team", "sre")
	require.NoError(t, err)
	assert.True(t, strings.Contains(out, "my-svc"),
		"stdout must mention the app name")
}

func TestInit_OutputMentionsTeam(t *testing.T) {
	dir := t.TempDir()
	out, _, err := runInitInDir(t, dir, "init", "--app", "my-svc", "--team", "sre")
	require.NoError(t, err)
	assert.True(t, strings.Contains(out, "sre"),
		"stdout must mention the team name")
}

func TestInit_NamespaceYamlContainsAppName(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runInitInDir(t, dir, "init", "--app", "payments", "--team", "finance")
	require.NoError(t, err)

	content, readErr := os.ReadFile(filepath.Join(dir, "infra", "payments", "namespace.yaml"))
	require.NoError(t, readErr)
	assert.True(t, strings.Contains(string(content), "payments"),
		"namespace.yaml must contain the app name")
}

func TestInit_PeerAuthYamlContainsMTLSStrict(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runInitInDir(t, dir, "init", "--app", "orders", "--team", "backend")
	require.NoError(t, err)

	content, readErr := os.ReadFile(filepath.Join(dir, "infra", "orders", "peer-authentication.yaml"))
	require.NoError(t, readErr)
	assert.True(t, strings.Contains(string(content), "STRICT"),
		"peer-authentication.yaml must enforce mTLS STRICT mode")
}

func TestInit_GatekeeperYamlContainsTeamLabel(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runInitInDir(t, dir, "init", "--app", "orders", "--team", "backend")
	require.NoError(t, err)

	content, readErr := os.ReadFile(filepath.Join(dir, "infra", "orders", "gatekeeper.yaml"))
	require.NoError(t, readErr)
	assert.True(t, strings.Contains(string(content), "team"),
		"gatekeeper.yaml must reference the 'team' label")
}

func TestInit_IsIdempotent(t *testing.T) {
	dir := t.TempDir()

	// First run.
	_, _, err := runInitInDir(t, dir, "init", "--app", "idempotent-svc", "--team", "ops")
	require.NoError(t, err)

	// Second run — must not fail.
	_, _, err = runInitInDir(t, dir, "init", "--app", "idempotent-svc", "--team", "ops")
	require.NoError(t, err, "running init twice must be idempotent")
}

// ── Missing required flags ────────────────────────────────────────────────────

func TestInit_MissingApp_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runInitInDir(t, dir, "init", "--team", "platform")
	require.Error(t, err, "missing --app must return an error")
}

func TestInit_MissingTeam_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runInitInDir(t, dir, "init", "--app", "test-app")
	require.Error(t, err, "missing --team must return an error")
}

func TestInit_MissingBothFlags_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runInitInDir(t, dir, "init")
	require.Error(t, err, "missing both flags must return an error")
}

// ── Input validation (delegated to k8s.Generate) ──────────────────────────────

func TestInit_InvalidAppName_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	// Uppercase is rejected by the RFC-1123 validator in internal/k8s.
	_, _, err := runInitInDir(t, dir, "init", "--app", "MyApp", "--team", "platform")
	require.Error(t, err, "uppercase app name must return a validation error")
}

func TestInit_NoResidueInOriginalDir(t *testing.T) {
	origDir := t.TempDir() // simulate the original working directory
	tempDir := t.TempDir() // isolated workspace for this run

	orig, _ := os.Getwd()
	_ = os.Chdir(origDir)
	t.Cleanup(func() { _ = os.Chdir(orig) })

	// Run init in tempDir — nothing should be written to origDir.
	_, _, _ = runInitInDir(t, tempDir, "init", "--app", "isolated-svc", "--team", "platform")
	_ = os.Chdir(origDir) // restore after runInitInDir's cleanup

	entries, err := os.ReadDir(origDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "no files must be written to the original working directory")
}
