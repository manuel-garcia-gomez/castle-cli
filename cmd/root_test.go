package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// executeRoot builds a fresh command tree, sets the provided args and
// captures stdout + stderr into buffers. It returns the combined buffers and
// the error returned by cobra's Execute().
func executeRoot(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer

	root := newRootCmd()
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SetArgs(args)

	err = root.Execute()
	return outBuf.String(), errBuf.String(), err
}

// ── Root command initialisation ───────────────────────────────────────────────

func TestRootCmd_Use(t *testing.T) {
	root := newRootCmd()
	assert.Equal(t, "castle", root.Use,
		"root command Use must be 'castle'")
}

func TestRootCmd_HasSubcommands(t *testing.T) {
	root := newRootCmd()
	names := make([]string, 0, len(root.Commands()))
	for _, c := range root.Commands() {
		names = append(names, c.Name())
	}
	assert.Contains(t, names, "init", "init sub-command must be registered")
	assert.Contains(t, names, "scan", "scan sub-command must be registered")
	assert.Contains(t, names, "deploy", "deploy sub-command must be registered")
}

func TestRootCmd_IndependentInstances(t *testing.T) {
	r1 := newRootCmd()
	r2 := newRootCmd()
	assert.NotSame(t, r1, r2, "newRootCmd must return independent instances")
}

// ── --help ────────────────────────────────────────────────────────────────────

func TestRootCmd_HelpFlag_NoError(t *testing.T) {
	_, _, err := executeRoot(t, "--help")
	// cobra returns nil for --help; the help text is written to stdout.
	assert.NoError(t, err, "--help must not return an error")
}

func TestRootCmd_HelpFlag_ContainsCastle(t *testing.T) {
	out, _, _ := executeRoot(t, "--help")
	assert.True(t, strings.Contains(out, "castle"),
		"--help output must mention 'castle'")
}

func TestRootCmd_HelpFlag_ListsSubcommands(t *testing.T) {
	out, _, _ := executeRoot(t, "--help")
	for _, sub := range []string{"init", "scan", "deploy"} {
		assert.True(t, strings.Contains(out, sub),
			"--help output must list sub-command %q", sub)
	}
}

// ── Unknown sub-command ───────────────────────────────────────────────────────

func TestRootCmd_UnknownSubcommand_ReturnsError(t *testing.T) {
	_, _, err := executeRoot(t, "doesnotexist")
	require.Error(t, err, "unknown sub-command must return an error")
}

// ── --config flag ─────────────────────────────────────────────────────────────

func TestRootCmd_ConfigFlag_Registered(t *testing.T) {
	root := newRootCmd()
	flag := root.PersistentFlags().Lookup("config")
	require.NotNil(t, flag, "--config persistent flag must be registered")
	assert.Equal(t, "string", flag.Value.Type())
}

// ── commandNameFromArgs ───────────────────────────────────────────────────────

func TestCommandNameFromArgs_ReturnsFirstPositional(t *testing.T) {
	assert.Equal(t, "scan", commandNameFromArgs([]string{"scan", "--type", "trivy"}))
	assert.Equal(t, "deploy", commandNameFromArgs([]string{"--config", "f.yaml", "deploy"}))
}

func TestCommandNameFromArgs_EmptyArgs_ReturnsUnknown(t *testing.T) {
	assert.Equal(t, "unknown", commandNameFromArgs(nil))
	assert.Equal(t, "unknown", commandNameFromArgs([]string{}))
	assert.Equal(t, "unknown", commandNameFromArgs([]string{"--flag"}))
}

// ── defaultRevision ───────────────────────────────────────────────────────────

func TestDefaultRevision_ProdEnvs(t *testing.T) {
	assert.Equal(t, "HEAD", defaultRevision("prod"))
	assert.Equal(t, "HEAD", defaultRevision("production"))
}

func TestDefaultRevision_NonProdEnvs(t *testing.T) {
	for _, env := range []string{"staging", "dev", "qa", ""} {
		assert.Equal(t, "main", defaultRevision(env),
			"env=%q must default to 'main'", env)
	}
}
