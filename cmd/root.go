package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/manuel-garcia-gomez/castle-cli/internal/config"
	"github.com/manuel-garcia-gomez/castle-cli/internal/telemetry"
	"github.com/spf13/cobra"
)

var (
	cfgFile string
	cfg     config.Config

	// metrics is initialised once in PersistentPreRunE and used in
	// PersistentPostRunE to record the command's outcome and duration.
	// It is a package-level variable so both hooks share the same instance.
	metrics *telemetry.Metrics

	// cmdStart records the wall-clock time at which PersistentPreRunE ran,
	// giving PersistentPostRunE a precise elapsed duration.
	cmdStart time.Time
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

	// PersistentPreRunE runs before every sub-command:
	//  1. Records the start time for later duration measurement.
	//  2. Initialises Prometheus metrics (isolated registry, never panics).
	//  3. Loads the configuration file.
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		cmdStart = time.Now()

		var err error
		metrics, err = telemetry.New()
		if err != nil {
			// Telemetry failure is non-fatal for a CLI tool; log and continue.
			slog.Warn("telemetry: failed to initialise metrics, continuing without",
				"error", err)
			metrics = nil
		}

		loaded, err := config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("initialising configuration: %w", err)
		}
		cfg = loaded

		slog.Info("castle starting",
			"command", cmd.Name(),
			"environment", cfg.Environment,
			"port", cfg.Port,
			"k8s_namespace", cfg.Kubernetes.Namespace,
		)
		return nil
	},

	// PersistentPostRunE runs after every sub-command that completed without
	// a fatal cobra error. It records the elapsed duration and a "success"
	// status. Commands that return an error bypass PostRun in cobra, so that
	// path is covered by PersistentPostRunEOnError via a wrapper — see below.
	PersistentPostRunE: func(cmd *cobra.Command, _ []string) error {
		recordCommandMetric(cmd.Name(), "success")
		return nil
	},
}

// recordCommandMetric is shared by the success and error paths so the logic
// lives in exactly one place.
func recordCommandMetric(cmdName, status string) {
	if metrics == nil {
		return
	}
	elapsed := time.Since(cmdStart)
	metrics.RecordCommand(cmdName, status, elapsed)
}

// Execute is the single entry-point called by main.
// It wraps rootCmd.Execute so that command errors are captured and forwarded
// to the telemetry layer before the process exits.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		// cobra already printed the error; record the metric then exit.
		// cmd.Name() is not available here, so we use the args-derived name.
		name := commandNameFromArgs(os.Args[1:])
		recordCommandMetric(name, "error")

		slog.Error("command failed", "error", err)
		os.Exit(1)
	}
}

// commandNameFromArgs extracts the first non-flag argument, which is the
// sub-command name (e.g. "scan", "deploy"). Falls back to "unknown".
func commandNameFromArgs(args []string) string {
	for _, a := range args {
		if len(a) > 0 && a[0] != '-' {
			return a
		}
	}
	return "unknown"
}

func init() {
	rootCmd.PersistentFlags().StringVar(
		&cfgFile,
		"config",
		"",
		"path to config file (default: $HOME/.castle.yaml or ./castle.yaml)",
	)
}
