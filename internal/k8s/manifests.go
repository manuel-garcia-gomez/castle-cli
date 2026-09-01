// Package k8s provides functions for generating Zero-Trust Kubernetes manifests
// that are ready to commit to a GitOps repository.
//
// Every generated YAML document is opinionated by design:
//   - Namespaces carry the "istio-injection: enabled" label so that the Istio
//     control-plane automatically injects sidecars into every workload.
//   - A PeerAuthentication policy enforces mTLS STRICT mode inside the
//     namespace, rejecting any plaintext traffic.
//   - An OPA Gatekeeper ConstraintTemplate + K8sRequiredLabels constraint
//     prevents workloads without a "team" label from being admitted.
package k8s

import (
	"bytes"
	"fmt"
	"regexp"
	"text/template"
)

// ─────────────────────────────────────────────────────────────────────────────
// Public types
// ─────────────────────────────────────────────────────────────────────────────

// Input holds the parameters required to render the full set of Zero-Trust
// manifests for a single microservice.
type Input struct {
	// App is the microservice name. It is used as the Kubernetes namespace
	// name, so it must comply with RFC 1123 label rules (lowercase
	// alphanumerics and hyphens, must start and end with an alphanumeric).
	App string

	// Team is the value written into the "team" label and enforced by the
	// OPA Gatekeeper K8sRequiredLabels constraint.
	Team string
}

// Manifest represents a single generated Kubernetes YAML document together
// with the filename under which it should be written to disk.
type Manifest struct {
	// Filename is the suggested file name (e.g. "namespace.yaml").
	Filename string
	// Content is the fully rendered YAML text.
	Content string
}

// ─────────────────────────────────────────────────────────────────────────────
// Public API
// ─────────────────────────────────────────────────────────────────────────────

// Generate validates in and returns the ordered set of Zero-Trust Kubernetes
// manifests that should be committed to the GitOps repository. The returned
// slice is ordered so that applying them with `kubectl apply -f .` works
// without dependency issues (Namespace → Istio → Gatekeeper).
func Generate(in Input) ([]Manifest, error) {
	if err := validate(in); err != nil {
		return nil, err
	}

	type entry struct {
		filename string
		tmpl     string
	}

	entries := []entry{
		{"namespace.yaml", namespaceTmpl},
		{"peer-authentication.yaml", peerAuthTmpl},
		{"gatekeeper.yaml", gatekeeperTmpl},
	}

	manifests := make([]Manifest, 0, len(entries))
	for _, e := range entries {
		content, err := render(e.tmpl, in)
		if err != nil {
			return nil, fmt.Errorf("k8s: rendering %s: %w", e.filename, err)
		}
		manifests = append(manifests, Manifest{
			Filename: e.filename,
			Content:  content,
		})
	}
	return manifests, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Validation
// ─────────────────────────────────────────────────────────────────────────────

// rfc1123Label matches valid Kubernetes name / namespace labels.
var rfc1123Label = regexp.MustCompile(`^[a-z0-9]([a-z0-9\-]*[a-z0-9])?$`)

// validate checks that all required fields are present and conform to the
// constraints imposed by Kubernetes naming rules.
func validate(in Input) error {
	if in.App == "" {
		return fmt.Errorf("k8s: app name must not be empty")
	}
	if !rfc1123Label.MatchString(in.App) {
		return fmt.Errorf(
			"k8s: app name %q is not a valid RFC 1123 label (lowercase alphanumerics and hyphens only, must start/end with alphanumeric)",
			in.App,
		)
	}
	if in.Team == "" {
		return fmt.Errorf("k8s: team must not be empty")
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Template rendering
// ─────────────────────────────────────────────────────────────────────────────

// render parses tmplSrc and executes it with data, returning the result as a
// string. Using text/template (not html/template) because the output is YAML,
// not HTML.
func render(tmplSrc string, data Input) (string, error) {
	t, err := template.New("manifest").Parse(tmplSrc)
	if err != nil {
		return "", fmt.Errorf("parsing template: %w", err)
	}
	var buf bytes.Buffer
	if err = t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}
	return buf.String(), nil
}

// ─────────────────────────────────────────────────────────────────────────────
// YAML templates
// ─────────────────────────────────────────────────────────────────────────────

// namespaceTmpl generates a Kubernetes Namespace with Istio sidecar injection
// enabled at the namespace level and a team label for policy enforcement.
const namespaceTmpl = `apiVersion: v1
kind: Namespace
metadata:
  name: {{ .App }}
  labels:
    # Required by Istio: enables automatic sidecar injection for every Pod.
    istio-injection: enabled
    app.kubernetes.io/name: {{ .App }}
    team: {{ .Team }}
`

// peerAuthTmpl generates an Istio PeerAuthentication policy that forces all
// intra-namespace traffic to use mTLS in STRICT mode. Workloads that do not
// present a valid client certificate will be rejected.
const peerAuthTmpl = `apiVersion: security.istio.io/v1beta1
kind: PeerAuthentication
metadata:
  name: default
  namespace: {{ .App }}
  labels:
    app.kubernetes.io/name: {{ .App }}
    team: {{ .Team }}
spec:
  mtls:
    # STRICT: Istio rejects any plaintext (non-mTLS) traffic to this namespace.
    mode: STRICT
`

// gatekeeperTmpl generates two concatenated resources separated by "---":
//
//  1. A ConstraintTemplate that defines the K8sRequiredLabels CRD and the Rego
//     policy logic evaluated by OPA at admission time.
//  2. A K8sRequiredLabels constraint instance that enforces the presence of
//     the "team" label on Pods, Deployments and Services in the namespace.
const gatekeeperTmpl = `apiVersion: templates.gatekeeper.sh/v1beta1
kind: ConstraintTemplate
metadata:
  name: k8srequiredlabels
  labels:
    team: {{ .Team }}
spec:
  crd:
    spec:
      names:
        kind: K8sRequiredLabels
      validation:
        openAPIV3Schema:
          type: object
          properties:
            labels:
              type: array
              items:
                type: string
  targets:
    - target: admission.k8s.gatekeeper.sh
      rego: |
        package k8srequiredlabels

        violation[{"msg": msg}] {
          provided := {label | input.review.object.metadata.labels[label]}
          required := {label | label := input.parameters.labels[_]}
          missing  := required - provided
          count(missing) > 0
          msg := sprintf("required labels are missing: %v", [missing])
        }
---
apiVersion: constraints.gatekeeper.sh/v1beta1
kind: K8sRequiredLabels
metadata:
  # Constraint name is scoped to the app to allow per-service overrides.
  name: require-team-label-{{ .App }}
  labels:
    app.kubernetes.io/name: {{ .App }}
    team: {{ .Team }}
spec:
  match:
    kinds:
      - apiGroups: [""]
        kinds: ["Pod", "Service"]
      - apiGroups: ["apps"]
        kinds: ["Deployment", "StatefulSet", "DaemonSet"]
    namespaces:
      - {{ .App }}
  parameters:
    labels:
      - team
`
