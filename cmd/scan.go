package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/manuel-garcia-gomez/castle-cli/internal/security"
	"github.com/spf13/cobra"
)

// newScanCmd builds a fresh "castle scan" command.
func newScanCmd() *cobra.Command {
	var (
		scanTarget       string
		scanType         string
		scanUpload       bool
		scanEngagementID int
	)

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Run a security scan and optionally upload results to DefectDojo",
		Long: `castle scan invokes a security scanner (trivy or semgrep) against the
specified target and writes findings to a temporary JSON file.

When --upload is set the report is forwarded to the DefectDojo instance
configured in castle.yaml (defectdojo.url / defectdojo.api_key) or via the
CASTLE_DEFECTDOJO_URL / CASTLE_DEFECTDOJO_API_KEY environment variables.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runScan(cmd, scanType, scanTarget, scanUpload, scanEngagementID)
		},
	}

	cmd.Flags().StringVar(&scanTarget, "target", ".",
		"path or container image to scan (default: current directory)")
	cmd.Flags().StringVar(&scanType, "type", "trivy",
		"scanner to use: trivy or semgrep")
	cmd.Flags().BoolVar(&scanUpload, "upload", false,
		"upload scan results to DefectDojo after the scan completes")
	cmd.Flags().IntVar(&scanEngagementID, "engagement-id", 1,
		"DefectDojo engagement ID to import findings into (required when --upload is set)")

	return cmd
}

// runScan contains the business logic for "castle scan".
func runScan(cmd *cobra.Command, scanType, scanTarget string, scanUpload bool, scanEngagementID int) error {
	ctx := context.Background()

	scanner, err := security.NewScanner(scanType)
	if err != nil {
		return fmt.Errorf("scan: %w", err)
	}

	slog.Info("scan: starting", "type", scanType, "target", scanTarget)

	reportPath, err := scanner.Scan(ctx, scanTarget)
	if err != nil {
		return fmt.Errorf("scan: running %s scanner: %w", scanType, err)
	}

	defer func() {
		if rmErr := os.Remove(reportPath); rmErr != nil && !os.IsNotExist(rmErr) {
			slog.Warn("scan: could not remove temporary report",
				"path", reportPath, "error", rmErr)
		}
	}()

	slog.Info("scan: finished", "report", reportPath)
	fmt.Fprintf(cmd.OutOrStdout(), "Scan complete. Report written to: %s\n", reportPath)

	if !scanUpload {
		return nil
	}

	if cfg.DefectDojo.APIKey == "" {
		return fmt.Errorf(
			"scan: --upload requires defectdojo.api_key; set it in castle.yaml or via CASTLE_DEFECTDOJO_API_KEY",
		)
	}

	ddScanType := defectDojoScanType(scanType)
	client := security.NewClient(cfg.DefectDojo.URL, cfg.DefectDojo.APIKey)

	slog.Info("scan: uploading to DefectDojo",
		"url", cfg.DefectDojo.URL,
		"dd_scan_type", ddScanType,
		"engagement_id", scanEngagementID,
	)

	result, err := client.UploadScan(ctx, scanEngagementID, ddScanType, reportPath)
	if err != nil {
		return fmt.Errorf("scan: uploading to DefectDojo: %w", err)
	}

	slog.Info("scan: upload successful", "test_id", result.TestID)
	fmt.Fprintf(cmd.OutOrStdout(), "Uploaded to DefectDojo. Test ID: %d\n", result.TestID)
	return nil
}

// defectDojoScanType maps the CLI --type flag value to the DefectDojo scan_type string.
func defectDojoScanType(t string) string {
	switch t {
	case "trivy":
		return "Trivy Scan"
	case "semgrep":
		return "Semgrep JSON Report"
	default:
		return t
	}
}
