package gitops_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/manuel-garcia-gomez/castle-cli/internal/gitops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

// validInput returns a fully populated AppInput that passes all validations.
func validInput() gitops.AppInput {
	return gitops.AppInput{
		App:            "payment-service",
		Env:            "staging",
		RepoURL:        "https://github.com/example/gitops.git",
		TargetRevision: "main",
		Path:           "infra/payment-service",
		Namespace:      "payment-service",
	}
}

// ─── YAML rendering ──────────────────────────────────────────────────────────

func TestGenerateAppManifest_IsArgoCDKind(t *testing.T) {
	yaml, err := gitops.GenerateAppManifest(validInput())
	require.NoError(t, err)
	assert.Contains(t, yaml, "apiVersion: argoproj.io/v1alpha1")
	assert.Contains(t, yaml, "kind: Application")
}

func TestGenerateAppManifest_AppNameCombinesAppAndEnv(t *testing.T) {
	yaml, err := gitops.GenerateAppManifest(validInput())
	require.NoError(t, err)
	assert.Contains(t, yaml, "name: payment-service-staging")
}

func TestGenerateAppManifest_LabelsContainAppAndEnv(t *testing.T) {
	yaml, err := gitops.GenerateAppManifest(validInput())
	require.NoError(t, err)
	assert.Contains(t, yaml, "app.kubernetes.io/name: payment-service")
	assert.Contains(t, yaml, "environment: staging")
}

func TestGenerateAppManifest_ContainsRepoURL(t *testing.T) {
	yaml, err := gitops.GenerateAppManifest(validInput())
	require.NoError(t, err)
	assert.Contains(t, yaml, "repoURL: https://github.com/example/gitops.git")
}

func TestGenerateAppManifest_ContainsTargetRevision(t *testing.T) {
	in := validInput()
	in.TargetRevision = "v2.1.0"
	yaml, err := gitops.GenerateAppManifest(in)
	require.NoError(t, err)
	assert.Contains(t, yaml, "targetRevision: v2.1.0")
}

func TestGenerateAppManifest_ContainsPath(t *testing.T) {
	yaml, err := gitops.GenerateAppManifest(validInput())
	require.NoError(t, err)
	assert.Contains(t, yaml, "path: infra/payment-service")
}

func TestGenerateAppManifest_ContainsDestinationNamespace(t *testing.T) {
	yaml, err := gitops.GenerateAppManifest(validInput())
	require.NoError(t, err)
	assert.Contains(t, yaml, "namespace: payment-service")
}

func TestGenerateAppManifest_HasAutomatedSyncPolicyWithPruneAndSelfHeal(t *testing.T) {
	yaml, err := gitops.GenerateAppManifest(validInput())
	require.NoError(t, err)
	assert.Contains(t, yaml, "automated:")
	assert.Contains(t, yaml, "prune: true")
	assert.Contains(t, yaml, "selfHeal: true")
	assert.Contains(t, yaml, "CreateNamespace=true")
}

func TestGenerateAppManifest_DifferentEnvs_ProduceIsolatedNames(t *testing.T) {
	stg := validInput()
	prod := validInput()
	prod.Env = "prod"
	prod.TargetRevision = "HEAD"

	yamlStg, err := gitops.GenerateAppManifest(stg)
	require.NoError(t, err)
	yamlProd, err := gitops.GenerateAppManifest(prod)
	require.NoError(t, err)

	assert.Contains(t, yamlStg, "payment-service-staging")
	assert.NotContains(t, yamlStg, "payment-service-prod")
	assert.Contains(t, yamlProd, "payment-service-prod")
	assert.NotContains(t, yamlProd, "payment-service-staging")
}

// ─── Validation errors ────────────────────────────────────────────────────────

func TestGenerateAppManifest_Error_EmptyApp(t *testing.T) {
	in := validInput()
	in.App = ""
	_, err := gitops.GenerateAppManifest(in)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "app name must not be empty")
}

func TestGenerateAppManifest_Error_EmptyEnv(t *testing.T) {
	in := validInput()
	in.Env = ""
	_, err := gitops.GenerateAppManifest(in)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "env must not be empty")
}

func TestGenerateAppManifest_Error_EmptyRepoURL(t *testing.T) {
	in := validInput()
	in.RepoURL = ""
	_, err := gitops.GenerateAppManifest(in)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "repoURL must not be empty")
}

func TestGenerateAppManifest_Error_EmptyTargetRevision(t *testing.T) {
	in := validInput()
	in.TargetRevision = ""
	_, err := gitops.GenerateAppManifest(in)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "targetRevision must not be empty")
}

func TestGenerateAppManifest_Error_EmptyPath(t *testing.T) {
	in := validInput()
	in.Path = ""
	_, err := gitops.GenerateAppManifest(in)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path must not be empty")
}

func TestGenerateAppManifest_Error_EmptyNamespace(t *testing.T) {
	in := validInput()
	in.Namespace = ""
	_, err := gitops.GenerateAppManifest(in)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "namespace must not be empty")
}

// ─── SyncApp HTTP client ──────────────────────────────────────────────────────

func TestSyncApp_Success200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v1/applications/payment-service-staging/sync", r.URL.Path)
		assert.Equal(t, "Bearer valid-token", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"metadata": {"name": "payment-service-staging"},
			"status":   {"sync": {"status": "Synced"}}
		}`))
	}))
	defer srv.Close()

	client := gitops.NewClient(srv.URL, "valid-token")
	resp, err := client.SyncApp(context.Background(), "payment-service-staging")

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "payment-service-staging", resp.Metadata.Name)
	assert.Equal(t, "Synced", resp.Status.Sync.Status)
}

func TestSyncApp_ServerError500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"internal server error"}`))
	}))
	defer srv.Close()

	client := gitops.NewClient(srv.URL, "token")
	_, err := client.SyncApp(context.Background(), "my-app-staging")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
	assert.Contains(t, err.Error(), "gitops: argocd:")
}

func TestSyncApp_Unauthorized401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Unauthorized"}`))
	}))
	defer srv.Close()

	client := gitops.NewClient(srv.URL, "bad-token")
	_, err := client.SyncApp(context.Background(), "my-app-staging")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")
	assert.Contains(t, err.Error(), "gitops: argocd:")
}

func TestSyncApp_EmptyAppName(t *testing.T) {
	// Should fail before any network call.
	client := gitops.NewClient("http://127.0.0.1:19999", "token")
	_, err := client.SyncApp(context.Background(), "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "app name must not be empty")
}

func TestSyncApp_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the call is made

	client := gitops.NewClient(srv.URL, "token")
	_, err := client.SyncApp(ctx, "my-app-staging")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "gitops: argocd:")
}
