package security

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const defaultHTTPTimeout = 30 * time.Second

// Client is an authenticated HTTP client for the DefectDojo API v2.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient returns a Client ready to talk to baseURL, authenticating every
// request with the given API key. A 30 s timeout is applied to all calls.
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: defaultHTTPTimeout,
		},
	}
}

// ImportScanResponse models the JSON body returned by /api/v2/import-scan/
// on a successful 201 Created.
type ImportScanResponse struct {
	TestID  int    `json:"test"`
	Message string `json:"message,omitempty"`
}

// UploadScan sends the file at filePath to DefectDojo's /api/v2/import-scan/
// endpoint as a multipart/form-data POST.
//
//   - engagementID: existing DefectDojo engagement that will contain the test.
//   - scanType:     DefectDojo-recognised string, e.g. "Trivy Scan".
//   - filePath:     local path to the JSON report produced by the scanner.
//
// Returns the parsed response or a wrapped, context-rich error.
func (c *Client) UploadScan(ctx context.Context, engagementID int, scanType, filePath string) (*ImportScanResponse, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("defectdojo: opening scan file %q: %w", filePath, err)
	}
	defer f.Close()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)

	// Mandatory fields for the DefectDojo import-scan endpoint.
	formFields := map[string]string{
		"scan_type":          scanType,
		"engagement":         fmt.Sprintf("%d", engagementID),
		"scan_date":          time.Now().Format("2006-01-02"),
		"minimum_severity":   "Info",
		"active":             "true",
		"verified":           "false",
		"close_old_findings": "false",
	}
	for key, val := range formFields {
		fw, fwErr := mw.CreateFormField(key)
		if fwErr != nil {
			return nil, fmt.Errorf("defectdojo: creating form field %q: %w", key, fwErr)
		}
		if _, wErr := io.WriteString(fw, val); wErr != nil {
			return nil, fmt.Errorf("defectdojo: writing form field %q: %w", key, wErr)
		}
	}

	fw, err := mw.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return nil, fmt.Errorf("defectdojo: creating form file part: %w", err)
	}
	if _, err = io.Copy(fw, f); err != nil {
		return nil, fmt.Errorf("defectdojo: copying scan file into request body: %w", err)
	}
	if err = mw.Close(); err != nil {
		return nil, fmt.Errorf("defectdojo: closing multipart writer: %w", err)
	}

	endpoint := c.baseURL + "/api/v2/import-scan/"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return nil, fmt.Errorf("defectdojo: building HTTP request: %w", err)
	}
	req.Header.Set("Authorization", "Token "+c.apiKey)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	slog.Info("defectdojo: uploading scan",
		"endpoint", endpoint,
		"scan_type", scanType,
		"engagement_id", engagementID,
		"file", filePath,
	)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("defectdojo: executing HTTP request: %w", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("defectdojo: reading response body: %w", err)
	}

	slog.Info("defectdojo: response received", "status_code", resp.StatusCode)

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("defectdojo: unexpected status %d: %s",
			resp.StatusCode, string(rawBody))
	}

	var result ImportScanResponse
	if err = json.Unmarshal(rawBody, &result); err != nil {
		return nil, fmt.Errorf("defectdojo: decoding response JSON: %w", err)
	}

	slog.Info("defectdojo: scan imported successfully", "test_id", result.TestID)
	return &result, nil
}
