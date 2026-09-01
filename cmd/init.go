package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/manuel-garcia-gomez/castle-cli/internal/k8s"
	"github.com/spf13/cobra"
)

// newInitCmd builds a fresh "castle init" command. Called by newRootCmd so
// every root tree gets independent flag state — safe for concurrent testing.
func newInitCmd() *cobra.Command {
	var initApp, initTeam string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold Zero-Trust Kubernetes manifests for a microservice",
		Long: `castle init generates a ready-to-commit set of opinionated Kubernetes
manifests under ./infra/<app>/ in the current directory:

  namespace.yaml           — Namespace with istio-injection: enabled
  peer-authentication.yaml — Istio PeerAuthentication enforcing mTLS STRICT
  gatekeeper.yaml          — OPA Gatekeeper ConstraintTemplate + K8sRequiredLabels
                             constraint that requires a "team" label on workloads

The generated files are idempotent: running castle init again with the same
flags overwrites the existing manifests with a fresh render.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runInit(cmd, initApp, initTeam)
		},
	}

	cmd.Flags().StringVar(&initApp, "app", "",
		"microservice name; used as the Kubernetes namespace (required)")
	cmd.Flags().StringVar(&initTeam, "team", "",
		"team that owns the microservice; written into Kubernetes labels (required)")

	_ = cmd.MarkFlagRequired("app")
	_ = cmd.MarkFlagRequired("team")

	return cmd
}

// runInit contains the business logic for "castle init".
// Separated from the cobra boilerplate so it can be called directly in tests.
func runInit(cmd *cobra.Command, initApp, initTeam string) error {
	manifests, err := k8s.Generate(k8s.Input{App: initApp, Team: initTeam})
	if err != nil {
		return fmt.Errorf("init: generating manifests: %w", err)
	}

	outDir := filepath.Join(".", "infra", initApp)
	if mkErr := os.MkdirAll(outDir, 0o755); mkErr != nil {
		return fmt.Errorf("init: creating output directory %q: %w", outDir, mkErr)
	}
	slog.Info("init: output directory ready", "path", outDir)

	for _, m := range manifests {
		dest := filepath.Join(outDir, m.Filename)
		if wErr := os.WriteFile(dest, []byte(m.Content), 0o644); wErr != nil {
			return fmt.Errorf("init: writing manifest %q: %w", dest, wErr)
		}
		slog.Info("init: manifest written", "file", dest)
		fmt.Fprintf(cmd.OutOrStdout(), "  created %s\n", dest)
	}

	fmt.Fprintf(cmd.OutOrStdout(),
		"\nZero-Trust manifests for %q (team: %q) written to %s\n"+
			"Review, adjust, and commit them to your GitOps repository.\n",
		initApp, initTeam, outDir,
	)
	return nil
}
