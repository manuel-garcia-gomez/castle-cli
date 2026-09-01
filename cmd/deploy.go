package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/manuel-garcia-gomez/castle-cli/internal/gitops"
	"github.com/spf13/cobra"
)

// newDeployCmd builds a fresh "castle deploy" command.
func newDeployCmd() *cobra.Command {
	var (
		deployApp      string
		deployEnv      string
		deployRepo     string
		deploySync     bool
		deployRevision string
	)

	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Generate an ArgoCD Application manifest and optionally trigger a sync",
		Long: `castle deploy generates the ArgoCD Application YAML for a microservice
and writes it to ./infra/<app>/argocd-app.yaml, ready to commit to your GitOps
repository.

The ArgoCD Application name is composed as <app>-<env>. The Git path defaults
to infra/<app>, matching the layout created by castle init.

Target revision defaults:
  staging / any non-prod env → main
  prod | production           → HEAD

Use --revision to override the default for that environment.

When --sync is set, castle deploy also calls the ArgoCD REST API to trigger an
immediate synchronisation. The ArgoCD server URL and API token are read from
the [argocd] section of castle.yaml, or from the CASTLE_ARGOCD_URL /
CASTLE_ARGOCD_TOKEN environment variables.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDeploy(cmd, deployApp, deployEnv, deployRepo, deployRevision, deploySync)
		},
	}

	cmd.Flags().StringVar(&deployApp, "app", "", "microservice name (required)")
	cmd.Flags().StringVar(&deployEnv, "env", "",
		`target environment, e.g. "staging" or "prod" (required)`)
	cmd.Flags().StringVar(&deployRepo, "repo", "",
		"Git repository URL that ArgoCD will track (required)")
	cmd.Flags().BoolVar(&deploySync, "sync", false,
		"trigger an immediate ArgoCD sync after writing the manifest")
	cmd.Flags().StringVar(&deployRevision, "revision", "",
		`Git revision to deploy (branch, tag or SHA); defaults to "main" for staging, "HEAD" for prod`)

	_ = cmd.MarkFlagRequired("app")
	_ = cmd.MarkFlagRequired("env")
	_ = cmd.MarkFlagRequired("repo")

	return cmd
}

// runDeploy contains the business logic for "castle deploy".
func runDeploy(cmd *cobra.Command, deployApp, deployEnv, deployRepo, deployRevision string, deploySync bool) error {
	revision := deployRevision
	if revision == "" {
		revision = defaultRevision(deployEnv)
	}

	appInput := gitops.AppInput{
		App:            deployApp,
		Env:            deployEnv,
		RepoURL:        deployRepo,
		TargetRevision: revision,
		Path:           fmt.Sprintf("infra/%s", deployApp),
		Namespace:      deployApp,
	}

	slog.Info("deploy: generating ArgoCD Application manifest",
		"app", deployApp, "env", deployEnv,
		"revision", revision, "repo", deployRepo,
	)

	content, err := gitops.GenerateAppManifest(appInput)
	if err != nil {
		return fmt.Errorf("deploy: generating ArgoCD manifest: %w", err)
	}

	outDir := filepath.Join(".", "infra", deployApp)
	if mkErr := os.MkdirAll(outDir, 0o755); mkErr != nil {
		return fmt.Errorf("deploy: creating output directory %q: %w", outDir, mkErr)
	}

	destPath := filepath.Join(outDir, "argocd-app.yaml")
	if wErr := os.WriteFile(destPath, []byte(content), 0o644); wErr != nil {
		return fmt.Errorf("deploy: writing ArgoCD manifest to %q: %w", destPath, wErr)
	}

	slog.Info("deploy: manifest written", "file", destPath)
	fmt.Fprintf(cmd.OutOrStdout(), "  created %s\n", destPath)

	if !deploySync {
		fmt.Fprintf(cmd.OutOrStdout(),
			"\nArgoCD Application manifest for %q (%s) written to %s\n"+
				"Commit it to your GitOps repo, or re-run with --sync to trigger an immediate sync.\n",
			deployApp, deployEnv, outDir,
		)
		return nil
	}

	if cfg.ArgoCD.URL == "" {
		return fmt.Errorf(
			"deploy: --sync requires argocd.url; set it in castle.yaml or via CASTLE_ARGOCD_URL",
		)
	}
	if cfg.ArgoCD.Token == "" {
		return fmt.Errorf(
			"deploy: --sync requires argocd.token; set it in castle.yaml or via CASTLE_ARGOCD_TOKEN",
		)
	}

	argoCDAppName := fmt.Sprintf("%s-%s", deployApp, deployEnv)
	client := gitops.NewClient(cfg.ArgoCD.URL, cfg.ArgoCD.Token)

	slog.Info("deploy: triggering ArgoCD sync",
		"argocd_url", cfg.ArgoCD.URL, "argocd_app", argoCDAppName)

	syncResp, err := client.SyncApp(context.Background(), argoCDAppName)
	if err != nil {
		return fmt.Errorf("deploy: triggering ArgoCD sync for %q: %w", argoCDAppName, err)
	}

	slog.Info("deploy: sync triggered",
		"app", argoCDAppName, "sync_status", syncResp.Status.Sync.Status)
	fmt.Fprintf(cmd.OutOrStdout(),
		"\nArgoCD sync triggered for %q. Sync status: %s\n",
		argoCDAppName, syncResp.Status.Sync.Status,
	)
	return nil
}

// defaultRevision maps environment names to Git refs.
func defaultRevision(env string) string {
	switch env {
	case "prod", "production":
		return "HEAD"
	default:
		return "main"
	}
}
