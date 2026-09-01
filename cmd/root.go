package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
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
	metrics *telemetry.Metrics

	// cmdStart records the wall-clock time at which PersistentPreRunE ran.
	cmdStart time.Time
)

// newRootCmd builds a fresh cobra command tree. Calling it more than once
// (e.g. in tests) is safe: every call returns independent instances with no
// shared flag state.
func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "castle",
		Short: "Castle CLI — self-service platform for deployments and security scans",
		Long: `Castle is a command-line tool that orchestrates Kubernetes deployments,
security scans via DefectDojo, and GitOps workflows from a single interface.

Configuration is loaded from castle.yaml (current directory or $HOME) and can
be overridden with environment variables prefixed with CASTLE_.`,
		SilenceUsage:  true,
		SilenceErrors: true,

		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			cmdStart = time.Now()

			var err error
			metrics, err = telemetry.New()
			if err != nil {
				slog.Warn("telemetry: failed to initialise metrics, continuing without",
					"error", err)
				metrics = nil
			}

			cfgPath, _ := cmd.Root().PersistentFlags().GetString("config")
			loaded, err := config.Load(cfgPath)
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

		PersistentPostRunE: func(cmd *cobra.Command, _ []string) error {
			recordCommandMetric(cmd.Name(), "success")
			return nil
		},
	}

	root.PersistentFlags().String(
		"config",
		"",
		"path to config file (default: $HOME/.castle.yaml or ./castle.yaml)",
	)

	root.AddCommand(newInitCmd())
	root.AddCommand(newScanCmd())
	root.AddCommand(newDeployCmd())

	return root
}

// rootCmd is the singleton used by Execute (the real process entry-point).
var rootCmd = newRootCmd()

// recordCommandMetric is shared by the success and error paths.
func recordCommandMetric(cmdName, status string) {
	if metrics == nil {
		return
	}
	elapsed := time.Since(cmdStart)
	metrics.RecordCommand(cmdName, status, elapsed)
}

// Execute is the single entry-point called by main.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		name := commandNameFromArgs(os.Args[1:])
		recordCommandMetric(name, "error")
		slog.Error("command failed", "error", err)
		os.Exit(1)
	}
}

// commandNameFromArgs extracts the first non-flag argument (the sub-command name).
// It skips flag values: --flag value and -f value are both handled correctly.
func commandNameFromArgs(args []string) string {
	skipNext := false
	for _, a := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if strings.HasPrefix(a, "-") {
			// --flag=value carries the value in the same token; no skip needed.
			// --flag value (two tokens): skip the next token.
			if strings.HasPrefix(a, "--") && !strings.Contains(a, "=") && len(a) > 2 {
				skipNext = true
			} else if !strings.HasPrefix(a, "--") && len(a) == 2 {
				// -f value (short flag, single char after dash): skip next.
				skipNext = true
			}
			continue
		}
		return a
	}
	return "unknown"
}
