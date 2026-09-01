package security

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"time"
)

// ScanType identifies a supported security scanner.
type ScanType string

const (
	ScanTypeTrivy   ScanType = "trivy"
	ScanTypeSemgrep ScanType = "semgrep"
)

// Scanner is the interface every security-scan adapter must implement.
// Scan executes the tool against target and returns the absolute path to a
// temporary JSON report file. The caller owns the file and must remove it
// when finished.
type Scanner interface {
	Scan(ctx context.Context, target string) (string, error)
}

// NewScanner returns the Scanner implementation for scanType.
// Returns an error for unrecognised values.
func NewScanner(scanType string) (Scanner, error) {
	switch ScanType(scanType) {
	case ScanTypeTrivy:
		return &TrivyScanner{}, nil
	case ScanTypeSemgrep:
		return &SemgrepScanner{}, nil
	default:
		return nil, fmt.Errorf("scanner: unsupported scan type %q; valid values: trivy, semgrep", scanType)
	}
}

// newOutputFile creates a uniquely-named temporary JSON file for scan output.
func newOutputFile(toolName string) (*os.File, error) {
	f, err := os.CreateTemp("", fmt.Sprintf("castle-%s-*.json", toolName))
	if err != nil {
		return nil, fmt.Errorf("scanner: creating temporary output file: %w", err)
	}
	return f, nil
}

// -----------------------------------------------------------------------------
// TrivyScanner
// -----------------------------------------------------------------------------

// TrivyScanner runs `trivy fs` against a filesystem target and writes the
// output as JSON. When the trivy binary cannot be found, a minimal stub
// report is produced so the rest of the pipeline can continue.
type TrivyScanner struct{}

func (s *TrivyScanner) Scan(ctx context.Context, target string) (string, error) {
	out, err := newOutputFile("trivy")
	if err != nil {
		return "", err
	}
	name := out.Name()
	out.Close() // trivy writes directly by path; close before handing it off

	slog.Info("scanner: starting Trivy scan", "target", target, "output", name)

	trivyPath, lookErr := exec.LookPath("trivy")
	if lookErr != nil {
		slog.Warn("scanner: trivy binary not found, generating stub report", "error", lookErr)
		if wErr := writeStubReport(name, "Trivy Scan", target); wErr != nil {
			return "", wErr
		}
		return name, nil
	}

	cmd := exec.CommandContext(ctx, trivyPath,
		"fs",
		"--format", "json",
		"--output", name,
		target,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if runErr := cmd.Run(); runErr != nil {
		return "", fmt.Errorf("scanner: trivy execution failed: %w", runErr)
	}

	slog.Info("scanner: Trivy scan complete", "output", name)
	return name, nil
}

// -----------------------------------------------------------------------------
// SemgrepScanner
// -----------------------------------------------------------------------------

// SemgrepScanner runs `semgrep --config auto` against a target directory and
// writes the output as JSON. Falls back to a stub report when semgrep is not
// installed.
type SemgrepScanner struct{}

func (s *SemgrepScanner) Scan(ctx context.Context, target string) (string, error) {
	out, err := newOutputFile("semgrep")
	if err != nil {
		return "", err
	}
	name := out.Name()
	out.Close()

	slog.Info("scanner: starting Semgrep scan", "target", target, "output", name)

	semgrepPath, lookErr := exec.LookPath("semgrep")
	if lookErr != nil {
		slog.Warn("scanner: semgrep binary not found, generating stub report", "error", lookErr)
		if wErr := writeStubReport(name, "Semgrep JSON Report", target); wErr != nil {
			return "", wErr
		}
		return name, nil
	}

	cmd := exec.CommandContext(ctx, semgrepPath,
		"--config", "auto",
		"--json",
		"--output", name,
		target,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if runErr := cmd.Run(); runErr != nil {
		return "", fmt.Errorf("scanner: semgrep execution failed: %w", runErr)
	}

	slog.Info("scanner: Semgrep scan complete", "output", name)
	return name, nil
}

// -----------------------------------------------------------------------------
// Stub report helpers
// -----------------------------------------------------------------------------

// stubReport is the minimal JSON document that acts as a placeholder when the
// scanner binary is unavailable. DefectDojo can ingest it as a generic import.
type stubReport struct {
	GeneratedAt string        `json:"generated_at"`
	Scanner     string        `json:"scanner"`
	Target      string        `json:"target"`
	Findings    []stubFinding `json:"findings"`
}

type stubFinding struct {
	Title       string `json:"title"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
}

// writeStubReport serialises an empty findings report to path.
func writeStubReport(path, scanner, target string) error {
	report := stubReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Scanner:     scanner,
		Target:      target,
		Findings:    []stubFinding{},
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("scanner: opening stub report file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err = enc.Encode(report); err != nil {
		return fmt.Errorf("scanner: writing stub report: %w", err)
	}
	return nil
}
