// Package gitops provides utilities for generating ArgoCD Application manifests
// and for interacting with the ArgoCD REST API.
//
// Typical workflow:
//
//  1. Call GenerateAppManifest to render the YAML file that describes the
//     ArgoCD Application; commit it to your GitOps repository.
//  2. Optionally call Client.SyncApp to trigger an immediate reconciliation
//     via the ArgoCD API, bypassing the default polling interval.
package gitops

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"text/template"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// Manifest generation
// ─────────────────────────────────────────────────────────────────────────────

// AppInput holds the parameters required to render an ArgoCD Application
// manifest. Every field is mandatory; GenerateAppManifest returns an error if
// any field is empty.
type AppInput struct {
	// App is the microservice name; combined with Env it forms the unique
	// ArgoCD Application name (<App>-<Env>).
	App string

	// Env is the target environment, e.g. "staging" or "prod". It is
	// embedded in the Application name and added as a label.
	Env string

	// RepoURL is the Git repository URL that ArgoCD will poll for changes.
	RepoURL string

	// TargetRevision is the Git branch, tag or commit SHA to track, e.g.
	// "main", "v1.4.2", or a full SHA.
	TargetRevision string

	// Path is the directory inside the repository that contains the
	// Kubernetes manifests ArgoCD should apply.
	Path string

	// Namespace is the destination Kubernetes namespace for the workloads.
	// It is written into spec.destination.namespace.
	Namespace string
}

// GenerateAppManifest validates in and returns the rendered ArgoCD Application
// YAML ready to be written to a GitOps repository.
func GenerateAppManifest(in AppInput) (string, error) {
	if err := validateAppInput(in); err != nil {
		return "", err
	}
	return renderTemplate(argoCDAppTmpl, in)
}

// validateAppInput returns an error for the first mandatory field found empty.
func validateAppInput(in AppInput) error {
	switch {
	case in.App == "":
		return fmt.Errorf("gitops: app name must not be empty")
	case in.Env == "":
		return fmt.Errorf("gitops: env must not be empty")
	case in.RepoURL == "":
		return fmt.Errorf("gitops: repoURL must not be empty")
	case in.TargetRevision == "":
		return fmt.Errorf("gitops: targetRevision must not be empty")
	case in.Path == "":
		return fmt.Errorf("gitops: path must not be empty")
	case in.Namespace == "":
		return fmt.Errorf("gitops: namespace must not be empty")
	}
	return nil
}

// renderTemplate parses tmplSrc and executes it with data, returning the
// rendered string. text/template is used because the output is YAML, not HTML.
func renderTemplate(tmplSrc string, data any) (string, error) {
	t, err := template.New("manifest").Parse(tmplSrc)
	if err != nil {
		return "", fmt.Errorf("gitops: parsing template: %w", err)
	}
	var buf bytes.Buffer
	if err = t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("gitops: executing template: %w", err)
	}
	return buf.String(), nil
}

// argoCDAppTmpl is the ArgoCD Application YAML template.
// It enables automated sync with pruning and self-healing so the cluster
// always converges to the declared state in Git.
const argoCDAppTmpl = `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  # Application name uniquely identifies this service+environment combination.
  name: {{ .App }}-{{ .Env }}
  # ArgoCD itself lives in the "argocd" namespace.
  namespace: argocd
  labels:
    app.kubernetes.io/name: {{ .App }}
    environment: {{ .Env }}
spec:
  project: default
  source:
    repoURL: {{ .RepoURL }}
    targetRevision: {{ .TargetRevision }}
    path: {{ .Path }}
  destination:
    server: https://kubernetes.default.svc
    namespace: {{ .Namespace }}
  syncPolicy:
    automated:
      # prune: delete resources removed from Git.
      prune: true
      # selfHeal: revert manual changes made directly in the cluster.
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
`

// ─────────────────────────────────────────────────────────────────────────────
// ArgoCD REST client
// ─────────────────────────────────────────────────────────────────────────────

const argoCDHTTPTimeout = 30 * time.Second

// Client is an authenticated HTTP client for the ArgoCD REST API.
// Requests are authenticated with a Bearer token.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient returns a Client that sends every request to baseURL authenticating
// with a Bearer token. A 30 s timeout is applied to all calls.
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: argoCDHTTPTimeout,
		},
	}
}

// SyncResponse contains the subset of the ArgoCD Application object returned
// by the sync endpoint that castle-cli cares about.
type SyncResponse struct {
	// Metadata carries the application identity echoed by ArgoCD.
	Metadata SyncMetadata `json:"metadata"`
	// Status reflects the outcome of the sync operation.
	Status SyncStatus `json:"status"`
}

// SyncMetadata mirrors the relevant subset of ArgoCD Application metadata.
type SyncMetadata struct {
	// Name is the ArgoCD Application name.
	Name string `json:"name"`
}

// SyncStatus mirrors the relevant subset of ArgoCD Application status.
type SyncStatus struct {
	// Sync contains the synchronisation state.
	Sync SyncState `json:"sync"`
}

// SyncState mirrors the ArgoCD sync state block.
type SyncState struct {
	// Status is one of: Synced | OutOfSync | Unknown.
	Status string `json:"status"`
}

// syncRequestBody is the JSON payload sent to POST /api/v1/applications/{name}/sync.
type syncRequestBody struct {
	// Revision pins the sync to a specific Git ref. An empty value tells
	// ArgoCD to use the Application's configured targetRevision.
	Revision string `json:"revision,omitempty"`
}

// SyncApp triggers a sync of the named ArgoCD application by calling
// POST /api/v1/applications/{appName}/sync.
//
// It returns the parsed SyncResponse on HTTP 200, or a wrapped error that
// includes the HTTP status code and response body for non-200 responses.
func (c *Client) SyncApp(ctx context.Context, appName string) (*SyncResponse, error) {
	if appName == "" {
		return nil, fmt.Errorf("gitops: argocd: app name must not be empty")
	}

	bodyBytes, err := json.Marshal(syncRequestBody{})
	if err != nil {
		return nil, fmt.Errorf("gitops: argocd: marshalling sync request body: %w", err)
	}

	endpoint := c.baseURL + "/api/v1/applications/" + appName + "/sync"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("gitops: argocd: building sync request for %q: %w", appName, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	slog.Info("gitops: argocd: triggering sync",
		"app", appName,
		"endpoint", endpoint,
	)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gitops: argocd: executing sync request for %q: %w", appName, err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gitops: argocd: reading response body: %w", err)
	}

	slog.Info("gitops: argocd: response received",
		"app", appName,
		"status_code", resp.StatusCode,
	)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"gitops: argocd: sync for %q returned unexpected status %d: %s",
			appName, resp.StatusCode, string(rawBody),
		)
	}

	var result SyncResponse
	if err = json.Unmarshal(rawBody, &result); err != nil {
		return nil, fmt.Errorf("gitops: argocd: decoding sync response: %w", err)
	}

	slog.Info("gitops: argocd: sync triggered successfully",
		"app", appName,
		"sync_status", result.Status.Sync.Status,
	)
	return &result, nil
}
