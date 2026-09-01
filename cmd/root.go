package cmd

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/manuel-garcia-gomez/castle-cli/internal/config"
	"github.com/spf13/cobra"
)

var (
	cfgFile string
	cfg     config.Config
)

// rootCmd is the base cobra command. All sub-commands attach here.
var rootCmd = &cobra.Command{
	Use:   "castle",
	Short: "Castle CLI — self-service platform for deployments and security scans",
	Long: `Castle is a command-line tool that orchestrates Kubernetes deployments,
security scans via DefectDojo, and GitOps workflows from a single interface.

Configuration is loaded from castle.yaml (current directory or $HOME) and can
be overridden with environment variables prefixed with CASTLE_.`,
	// SilenceUsage avoids printing the usage block on runtime errors.
	SilenceUsage: true,
	// PersistentPreRunE runs before every sub-command, ensuring config is
	// always initialised before any business logic executes.
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		loaded, err := config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("initialising configuration: %w", err)
		}
		cfg = loaded

		slog.Info("castle starting",
			"environment", cfg.Environment,
			"port", cfg.Port,
			"k8s_namespace", cfg.Kubernetes.Namespace,
		)
		return nil
	},
}

// Execute is the single entry-point called by main.
// On failure it logs the error and exits with code 1 — no panics.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		slog.Error("command failed", "error", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(
		&cfgFile,
		"config",
		"",
		"path to config file (default: $HOME/.castle.yaml or ./castle.yaml)",
	)
}
