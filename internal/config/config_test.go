package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/manuel-garcia-gomez/castle-cli/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_Defaults(t *testing.T) {
	// No config file anywhere → Load must succeed and return built-in defaults.
	cfg, err := config.Load("")
	require.NoError(t, err)

	assert.Equal(t, "development", cfg.Environment)
	assert.Equal(t, 8080, cfg.Port)
	assert.Equal(t, "http://localhost:8000", cfg.DefectDojo.URL)
	assert.Equal(t, "", cfg.DefectDojo.APIKey)
	assert.Equal(t, "default", cfg.Kubernetes.Namespace)
	assert.Equal(t, "", cfg.Kubernetes.Kubeconfig)
}

func TestLoad_FromYAML(t *testing.T) {
	yaml := `
environment: staging
port: 9090
defectdojo:
  url: https://dd.example.com
  api_key: secret-key-123
kubernetes:
  namespace: staging-ns
  kubeconfig: /home/user/.kube/config
`
	cfgPath := writeTempYAML(t, yaml)

	cfg, err := config.Load(cfgPath)
	require.NoError(t, err)

	assert.Equal(t, "staging", cfg.Environment)
	assert.Equal(t, 9090, cfg.Port)
	assert.Equal(t, "https://dd.example.com", cfg.DefectDojo.URL)
	assert.Equal(t, "secret-key-123", cfg.DefectDojo.APIKey)
	assert.Equal(t, "staging-ns", cfg.Kubernetes.Namespace)
	assert.Equal(t, "/home/user/.kube/config", cfg.Kubernetes.Kubeconfig)
}

func TestLoad_PartialYAML_FallsBackToDefaults(t *testing.T) {
	// Only override environment; remaining keys must retain their defaults.
	yaml := "environment: production\n"
	cfgPath := writeTempYAML(t, yaml)

	cfg, err := config.Load(cfgPath)
	require.NoError(t, err)

	assert.Equal(t, "production", cfg.Environment)
	assert.Equal(t, 8080, cfg.Port)
	assert.Equal(t, "http://localhost:8000", cfg.DefectDojo.URL)
	assert.Equal(t, "default", cfg.Kubernetes.Namespace)
}

func TestLoad_ExplicitFilePath_NotFound(t *testing.T) {
	_, err := config.Load("/nonexistent/path/castle.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "config:")
}

func TestLoad_InvalidYAML(t *testing.T) {
	cfgPath := writeTempYAML(t, ":::not: valid: yaml:::")

	_, err := config.Load(cfgPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "config:")
}

func TestLoad_EnvVarOverride(t *testing.T) {
	t.Setenv("CASTLE_ENVIRONMENT", "ci")
	t.Setenv("CASTLE_PORT", "7777")

	cfg, err := config.Load("")
	require.NoError(t, err)

	assert.Equal(t, "ci", cfg.Environment)
	assert.Equal(t, 7777, cfg.Port)
}

// writeTempYAML creates a temporary castle.yaml and returns its path.
func writeTempYAML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "castle.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}
