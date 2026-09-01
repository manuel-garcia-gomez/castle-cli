package k8s_test

import (
	"strings"
	"testing"

	"github.com/manuel-garcia-gomez/castle-cli/internal/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helper: generate manifests for a happy-path input and assert no error.
func mustGenerate(t *testing.T, app, team string) []k8s.Manifest {
	t.Helper()
	manifests, err := k8s.Generate(k8s.Input{App: app, Team: team})
	require.NoError(t, err)
	require.Len(t, manifests, 3, "expected exactly 3 manifests (namespace, peer-auth, gatekeeper)")
	return manifests
}

// contentOf returns the Content of the first Manifest whose Filename matches
// the given name, failing the test if none is found.
func contentOf(t *testing.T, manifests []k8s.Manifest, filename string) string {
	t.Helper()
	for _, m := range manifests {
		if m.Filename == filename {
			return m.Content
		}
	}
	t.Fatalf("manifest %q not found in generated set", filename)
	return ""
}

// ─── Namespace ───────────────────────────────────────────────────────────────

func TestGenerate_Namespace_ContainsAppName(t *testing.T) {
	manifests := mustGenerate(t, "payment-service", "platform")
	ns := contentOf(t, manifests, "namespace.yaml")

	assert.Contains(t, ns, "name: payment-service")
	assert.Contains(t, ns, "app.kubernetes.io/name: payment-service")
}

func TestGenerate_Namespace_HasIstioInjectionLabel(t *testing.T) {
	manifests := mustGenerate(t, "order-service", "commerce")
	ns := contentOf(t, manifests, "namespace.yaml")

	assert.Contains(t, ns, "istio-injection: enabled")
}

func TestGenerate_Namespace_HasTeamLabel(t *testing.T) {
	manifests := mustGenerate(t, "inventory", "logistics")
	ns := contentOf(t, manifests, "namespace.yaml")

	assert.Contains(t, ns, "team: logistics")
}

// ─── PeerAuthentication ───────────────────────────────────────────────────────

func TestGenerate_PeerAuthentication_IsIstioKind(t *testing.T) {
	manifests := mustGenerate(t, "auth-service", "identity")
	pa := contentOf(t, manifests, "peer-authentication.yaml")

	assert.Contains(t, pa, "kind: PeerAuthentication")
	assert.Contains(t, pa, "apiVersion: security.istio.io/v1beta1")
}

func TestGenerate_PeerAuthentication_StrictMode(t *testing.T) {
	manifests := mustGenerate(t, "auth-service", "identity")
	pa := contentOf(t, manifests, "peer-authentication.yaml")

	assert.Contains(t, pa, "mode: STRICT",
		"PeerAuthentication must enforce mTLS STRICT mode")
}

func TestGenerate_PeerAuthentication_NamespaceMatchesApp(t *testing.T) {
	manifests := mustGenerate(t, "billing", "finance")
	pa := contentOf(t, manifests, "peer-authentication.yaml")

	assert.Contains(t, pa, "namespace: billing")
}

// ─── Gatekeeper ──────────────────────────────────────────────────────────────

func TestGenerate_Gatekeeper_ContainsConstraintTemplate(t *testing.T) {
	manifests := mustGenerate(t, "shipping", "ops")
	gk := contentOf(t, manifests, "gatekeeper.yaml")

	assert.Contains(t, gk, "kind: ConstraintTemplate")
	assert.Contains(t, gk, "kind: K8sRequiredLabels")
}

func TestGenerate_Gatekeeper_ConstraintScopedToApp(t *testing.T) {
	manifests := mustGenerate(t, "shipping", "ops")
	gk := contentOf(t, manifests, "gatekeeper.yaml")

	assert.Contains(t, gk, "name: require-team-label-shipping")
	assert.Contains(t, gk, "- shipping")
}

func TestGenerate_Gatekeeper_EnforcesTeamLabel(t *testing.T) {
	manifests := mustGenerate(t, "shipping", "ops")
	gk := contentOf(t, manifests, "gatekeeper.yaml")

	// The Rego policy and the parameters section both reference "team".
	assert.Contains(t, gk, `- team`)
}

func TestGenerate_Gatekeeper_HasRegoPolicy(t *testing.T) {
	manifests := mustGenerate(t, "gateway", "platform")
	gk := contentOf(t, manifests, "gatekeeper.yaml")

	assert.Contains(t, gk, "package k8srequiredlabels")
	assert.Contains(t, gk, "violation")
}

// ─── Filenames ────────────────────────────────────────────────────────────────

func TestGenerate_Filenames(t *testing.T) {
	manifests := mustGenerate(t, "my-app", "my-team")
	names := make([]string, len(manifests))
	for i, m := range manifests {
		names[i] = m.Filename
	}
	assert.Equal(t, []string{"namespace.yaml", "peer-authentication.yaml", "gatekeeper.yaml"}, names)
}

// ─── Validation ───────────────────────────────────────────────────────────────

func TestGenerate_Error_EmptyApp(t *testing.T) {
	_, err := k8s.Generate(k8s.Input{App: "", Team: "platform"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "app name must not be empty")
}

func TestGenerate_Error_EmptyTeam(t *testing.T) {
	_, err := k8s.Generate(k8s.Input{App: "my-app", Team: ""})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "team must not be empty")
}

func TestGenerate_Error_InvalidAppName_UpperCase(t *testing.T) {
	_, err := k8s.Generate(k8s.Input{App: "MyApp", Team: "platform"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a valid RFC 1123 label")
}

func TestGenerate_Error_InvalidAppName_LeadingHyphen(t *testing.T) {
	_, err := k8s.Generate(k8s.Input{App: "-bad-name", Team: "platform"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a valid RFC 1123 label")
}

func TestGenerate_Error_InvalidAppName_Underscore(t *testing.T) {
	_, err := k8s.Generate(k8s.Input{App: "bad_name", Team: "platform"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a valid RFC 1123 label")
}

// ─── Template isolation ────────────────────────────────────────────────────────

func TestGenerate_DifferentApps_ProduceIsolatedManifests(t *testing.T) {
	m1 := mustGenerate(t, "service-a", "team-alpha")
	m2 := mustGenerate(t, "service-b", "team-beta")

	ns1 := contentOf(t, m1, "namespace.yaml")
	ns2 := contentOf(t, m2, "namespace.yaml")

	assert.True(t, strings.Contains(ns1, "service-a") && !strings.Contains(ns1, "service-b"),
		"namespace.yaml for service-a must not leak service-b values")
	assert.True(t, strings.Contains(ns2, "service-b") && !strings.Contains(ns2, "service-a"),
		"namespace.yaml for service-b must not leak service-a values")
}
