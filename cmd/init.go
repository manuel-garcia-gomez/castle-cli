package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/manuel-garcia-gomez/castle-cli/internal/k8s"
	"github.com/spf13/cobra"
)

var (
	initApp  string
	initTeam string
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Scaffold Zero-Trust Kubernetes manifests for a microservice",
	Long: `castle init generates a ready-to-commit set of opinionated Kubernetes
manifests under ./infra/<app>/ in the current directory:

  namespace.yaml          — Namespace with istio-injection: enabled
  peer-authentication.yaml — Istio PeerAuthentication enforcing mTLS STRICT
  gatekeeper.yaml         — OPA Gatekeeper ConstraintTemplate + K8sRequiredLabels
                            constraint that requires a "team" label on workloads

The generated files are idempotent: running castle init again with the same
flags overwrites the existing manifests with a fresh render.`,
	SilenceUsage: true,
	RunE:         runInit,
}

func runInit(cmd *cobra.Command, _ []string) error {
	// --- 1. Generate manifests ---
	manifests, err := k8s.Generate(k8s.Input{
		App:  initApp,
		Team: initTeam,
	})
	if err != nil {
		return fmt.Errorf("init: generating manifests: %w", err)
	}

	// --- 2. Create output directory ./infra/<app>/ ---
	outDir := filepath.Join(".", "infra", initApp)
	if mkErr := os.MkdirAll(outDir, 0o755); mkErr != nil {
		return fmt.Errorf("init: creating output directory %q: %w", outDir, mkErr)
	}
	slog.Info("init: output directory ready", "path", outDir)

	// --- 3. Write each manifest to disk ---
	for _, m := range manifests {
		dest := filepath.Join(outDir, m.Filename)
		if wErr := os.WriteFile(dest, []byte(m.Content), 0o644); wErr != nil {
			return fmt.Errorf("init: writing manifest %q: %w", dest, wErr)
		}
		slog.Info("init: manifest written", "file", dest)
		fmt.Fprintf(cmd.OutOrStdout(), "  created %s\n", dest)
	}

	// --- 4. Report summary ---
	fmt.Fprintf(cmd.OutOrStdout(),
		"\nZero-Trust manifests for %q (team: %q) written to %s\n"+
			"Review, adjust, and commit them to your GitOps repository.\n",
		initApp, initTeam, outDir,
	)
	return nil
}

func init() {
	initCmd.Flags().StringVar(
		&initApp, "app", "",
		"microservice name; used as the Kubernetes namespace (required)",
	)
	initCmd.Flags().StringVar(
		&initTeam, "team", "",
		"team that owns the microservice; written into Kubernetes labels (required)",
	)

	// Mark both flags as required so cobra surfaces a clear error when omitted.
	_ = initCmd.MarkFlagRequired("app")
	_ = initCmd.MarkFlagRequired("team")

	rootCmd.AddCommand(initCmd)
}
