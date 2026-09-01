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

// runDeployInDir executes "castle deploy" inside dir (as working directory)
// with the supplied args, capturing stdout and stderr.
func runDeployInDir(t *testing.T, dir string, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	orig, wdErr := os.Getwd()
	require.NoError(t, wdErr)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(orig) })

	var outBuf, errBuf bytes.Buffer
	root := newRootCmd()
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SetArgs(args)

	err = root.Execute()
	return outBuf.String(), errBuf.String(), err
}

const testRepo = "https://github.com/org/repo.git"

// ── Happy path — manifest creation ───────────────────────────────────────────

func TestDeploy_CreatesArgoCDManifest(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runDeployInDir(t, dir,
		"deploy", "--app", "test-app", "--env", "staging", "--repo", testRepo)
	require.NoError(t, err)

	manifest := filepath.Join(dir, "infra", "test-app", "argocd-app.yaml")
	info, statErr := os.Stat(manifest)
	require.NoError(t, statErr, "argocd-app.yaml must be created")
	assert.Greater(t, info.Size(), int64(0), "argocd-app.yaml must not be empty")
}

func TestDeploy_ManifestContainsAppName(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runDeployInDir(t, dir,
		"deploy", "--app", "payments", "--env", "staging", "--repo", testRepo)
	require.NoError(t, err)

	content, readErr := os.ReadFile(filepath.Join(dir, "infra", "payments", "argocd-app.yaml"))
	require.NoError(t, readErr)
	assert.True(t, strings.Contains(string(content), "payments"),
		"manifest must contain the app name")
}

func TestDeploy_ManifestContainsEnvInAppName(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runDeployInDir(t, dir,
		"deploy", "--app", "orders", "--env", "staging", "--repo", testRepo)
	require.NoError(t, err)

	content, readErr := os.ReadFile(filepath.Join(dir, "infra", "orders", "argocd-app.yaml"))
	require.NoError(t, readErr)
	// ArgoCD app name convention: <app>-<env>
	assert.True(t, strings.Contains(string(content), "orders-staging"),
		"manifest must contain the ArgoCD application name '<app>-<env>'")
}

func TestDeploy_ManifestContainsRepoURL(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runDeployInDir(t, dir,
		"deploy", "--app", "svc", "--env", "staging", "--repo", testRepo)
	require.NoError(t, err)

	content, readErr := os.ReadFile(filepath.Join(dir, "infra", "svc", "argocd-app.yaml"))
	require.NoError(t, readErr)
	assert.True(t, strings.Contains(string(content), testRepo),
		"manifest must contain the repo URL")
}

func TestDeploy_ManifestContainsAutoSync(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runDeployInDir(t, dir,
		"deploy", "--app", "svc", "--env", "staging", "--repo", testRepo)
	require.NoError(t, err)

	content, readErr := os.ReadFile(filepath.Join(dir, "infra", "svc", "argocd-app.yaml"))
	require.NoError(t, readErr)
	assert.True(t, strings.Contains(string(content), "automated"),
		"manifest must declare automated sync policy")
}

// ── Default revision policy ───────────────────────────────────────────────────

func TestDeploy_StagingUsesMainRevision(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runDeployInDir(t, dir,
		"deploy", "--app", "svc", "--env", "staging", "--repo", testRepo)
	require.NoError(t, err)

	content, _ := os.ReadFile(filepath.Join(dir, "infra", "svc", "argocd-app.yaml"))
	assert.True(t, strings.Contains(string(content), "main"),
		"staging must default to 'main' revision")
}

func TestDeploy_ProdUsesHEADRevision(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runDeployInDir(t, dir,
		"deploy", "--app", "svc", "--env", "prod", "--repo", testRepo)
	require.NoError(t, err)

	content, _ := os.ReadFile(filepath.Join(dir, "infra", "svc", "argocd-app.yaml"))
	assert.True(t, strings.Contains(string(content), "HEAD"),
		"prod must default to 'HEAD' revision")
}

func TestDeploy_ExplicitRevisionOverridesDefault(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runDeployInDir(t, dir,
		"deploy", "--app", "svc", "--env", "staging", "--repo", testRepo, "--revision", "v1.2.3")
	require.NoError(t, err)

	content, _ := os.ReadFile(filepath.Join(dir, "infra", "svc", "argocd-app.yaml"))
	assert.True(t, strings.Contains(string(content), "v1.2.3"),
		"explicit --revision must appear in the manifest")
}

// ── Missing required flags ────────────────────────────────────────────────────

func TestDeploy_MissingApp_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runDeployInDir(t, dir,
		"deploy", "--env", "staging", "--repo", testRepo)
	require.Error(t, err, "missing --app must return an error")
}

func TestDeploy_MissingEnv_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runDeployInDir(t, dir,
		"deploy", "--app", "svc", "--repo", testRepo)
	require.Error(t, err, "missing --env must return an error")
}

func TestDeploy_MissingRepo_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runDeployInDir(t, dir,
		"deploy", "--app", "svc", "--env", "staging")
	require.Error(t, err, "missing --repo must return an error")
}

// ── --sync without ArgoCD config ─────────────────────────────────────────────

func TestDeploy_Sync_WithoutArgoCDURL_ReturnsError(t *testing.T) {
	// Use env vars so config.Load() in PersistentPreRunE picks them up.
	// CASTLE_ARGOCD_URL not set → defaults to "" → url check fails.
	t.Setenv("CASTLE_ARGOCD_TOKEN", "some-token")

	dir := t.TempDir()
	_, _, err := runDeployInDir(t, dir,
		"deploy", "--app", "svc", "--env", "staging", "--repo", testRepo, "--sync")
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "argocd.url"),
		"error must mention argocd.url")
}

func TestDeploy_Sync_WithoutArgoCDToken_ReturnsError(t *testing.T) {
	// Use env vars so config.Load() in PersistentPreRunE picks them up.
	// CASTLE_ARGOCD_TOKEN not set → defaults to "" → token check fails.
	t.Setenv("CASTLE_ARGOCD_URL", "https://argocd.example.com")

	dir := t.TempDir()
	_, _, err := runDeployInDir(t, dir,
		"deploy", "--app", "svc", "--env", "staging", "--repo", testRepo, "--sync")
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "argocd.token"),
		"error must mention argocd.token")
}

// ── No residue outside temp dir ───────────────────────────────────────────────

func TestDeploy_NoResidueInOriginalDir(t *testing.T) {
	origDir := t.TempDir()
	workDir := t.TempDir()

	orig, _ := os.Getwd()
	_ = os.Chdir(origDir)
	t.Cleanup(func() { _ = os.Chdir(orig) })

	_, _, _ = runDeployInDir(t, workDir,
		"deploy", "--app", "isolated", "--env", "staging", "--repo", testRepo)
	_ = os.Chdir(origDir)

	entries, err := os.ReadDir(origDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "no files must be written to the original working directory")
}
