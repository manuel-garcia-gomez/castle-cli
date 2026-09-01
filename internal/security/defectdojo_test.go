package security_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/manuel-garcia-gomez/castle-cli/internal/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tempScanFile creates a minimal JSON report file and registers its cleanup.
func tempScanFile(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp("", "castle-test-scan-*.json")
	require.NoError(t, err)
	_, err = f.WriteString(`{"findings":[]}`)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	t.Cleanup(func() { os.Remove(f.Name()) })
	return f.Name()
}

// TestUploadScan_Success201 verifies that a 201 Created response is parsed
// correctly and the caller receives the test ID.
func TestUploadScan_Success201(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v2/import-scan/", r.URL.Path)
		assert.Equal(t, "Token valid-key", r.Header.Get("Authorization"))
		assert.Contains(t, r.Header.Get("Content-Type"), "multipart/form-data")

		require.NoError(t, r.ParseMultipartForm(10<<20))
		assert.Equal(t, "Trivy Scan", r.FormValue("scan_type"))
		assert.Equal(t, "7", r.FormValue("engagement"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"test": 99}`))
	}))
	defer srv.Close()

	client := security.NewClient(srv.URL, "valid-key")
	resp, err := client.UploadScan(context.Background(), 7, "Trivy Scan", tempScanFile(t))

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 99, resp.TestID)
}

// TestUploadScan_Unauthorized401 verifies that a 401 response surfaces as a
// descriptive error containing the status code.
func TestUploadScan_Unauthorized401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"detail":"Invalid token."}`))
	}))
	defer srv.Close()

	client := security.NewClient(srv.URL, "bad-key")
	_, err := client.UploadScan(context.Background(), 1, "Trivy Scan", tempScanFile(t))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")
	assert.Contains(t, err.Error(), "defectdojo:")
}

// TestUploadScan_ServerError500 verifies that a 500 response is treated as a
// failure and the error carries the status code.
func TestUploadScan_ServerError500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`internal server error`))
	}))
	defer srv.Close()

	client := security.NewClient(srv.URL, "any-key")
	_, err := client.UploadScan(context.Background(), 1, "Trivy Scan", tempScanFile(t))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

// TestUploadScan_FileNotFound verifies that attempting to upload a non-existent
// file returns a wrapped error before any network call is made.
func TestUploadScan_FileNotFound(t *testing.T) {
	client := security.NewClient("http://127.0.0.1:19999", "key")
	_, err := client.UploadScan(context.Background(), 1, "Trivy Scan", "/nonexistent/report.json")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "opening scan file")
}

// TestUploadScan_ContextCancelled verifies that a pre-cancelled context causes
// the request to fail without hanging.
func TestUploadScan_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"test":1}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the request is sent

	client := security.NewClient(srv.URL, "key")
	_, err := client.UploadScan(ctx, 1, "Trivy Scan", tempScanFile(t))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "defectdojo:")
}
