package gzip

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStaticFileWithGzip(t *testing.T) {
	// Create a temporary directory and file for testing
	tmpDir, err := os.MkdirTemp("", "gzip_static_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "This is a test file for static gzip compression testing. It should be long enough to trigger gzip compression."
	err = os.WriteFile(testFile, []byte(testContent), 0644)
	require.NoError(t, err)

	// Set up Gin router with gzip middleware and static file serving
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Gzip(DefaultCompression))
	router.Static("/static", tmpDir)

	// Test static file request with gzip support
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/static/test.txt", nil)
	req.Header.Add(headerAcceptEncoding, "gzip")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// The response should be successful and compressed
	assert.Equal(t, http.StatusOK, w.Code)

	// This is what should happen but currently fails due to the bug
	// The static handler initially sets status to 404, causing gzip headers to be removed
	assert.Equal(t, "gzip", w.Header().Get(headerContentEncoding), "Static file should be gzip compressed")
	assert.Equal(t, headerAcceptEncoding, w.Header().Get(headerVary), "Vary header should be set")

	// The compressed content should be smaller than original
	assert.Less(t, w.Body.Len(), len(testContent), "Compressed content should be smaller")
}

func TestStaticFileWithoutGzip(t *testing.T) {
	// Create a temporary directory and file for testing
	tmpDir, err := os.MkdirTemp("", "gzip_static_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "This is a test file."
	err = os.WriteFile(testFile, []byte(testContent), 0644)
	require.NoError(t, err)

	// Set up Gin router with gzip middleware and static file serving
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Gzip(DefaultCompression))
	router.Static("/static", tmpDir)

	// Test static file request without gzip support
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/static/test.txt", nil)
	// No Accept-Encoding header

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// The response should be successful and not compressed
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "", w.Header().Get(headerContentEncoding), "Content should not be compressed")
	assert.Equal(t, "", w.Header().Get(headerVary), "Vary header should not be set")
	assert.Equal(t, testContent, w.Body.String(), "Content should match original")
}

func TestStaticFileNotFound(t *testing.T) {
	// Create a temporary directory (but no files)
	tmpDir, err := os.MkdirTemp("", "gzip_static_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Set up Gin router with gzip middleware and static file serving
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Gzip(DefaultCompression))
	router.Static("/static", tmpDir)

	// Test request for non-existent file
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/static/nonexistent.txt", nil)
	req.Header.Add(headerAcceptEncoding, "gzip")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// The response should be 404 and not compressed (this should work correctly)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "", w.Header().Get(headerContentEncoding), "404 response should not be compressed")
	assert.Equal(t, "", w.Header().Get(headerVary), "Vary header should be removed for error responses")
}

func TestStaticDirectoryListing(t *testing.T) {
	// Create a temporary directory with a file
	tmpDir, err := os.MkdirTemp("", "gzip_static_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err)

	// Set up Gin router with gzip middleware and static file serving
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Gzip(DefaultCompression))
	router.Static("/static", tmpDir)

	// Test directory listing request
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/static/", nil)
	req.Header.Add(headerAcceptEncoding, "gzip")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Note: Gin's default static handler doesn't enable directory listing
	// so this will return 404, which should NOT be compressed
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "", w.Header().Get(headerContentEncoding), "404 response should not be compressed")
	assert.Equal(t, "", w.Header().Get(headerVary), "Vary header should be removed for error responses")
}

// This test demonstrates the specific issue mentioned in #122
func TestStaticFileGzipHeadersBug(t *testing.T) {
	// Create a temporary directory and file for testing
	tmpDir, err := os.MkdirTemp("", "gzip_static_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.js")
	testContent := "console.log('This is a JavaScript file that should be compressed when served as a static file');"
	err = os.WriteFile(testFile, []byte(testContent), 0644)
	require.NoError(t, err)

	// Set up Gin router with gzip middleware and static file serving
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Gzip(DefaultCompression))
	router.Static("/assets", tmpDir)

	// Test static file request with gzip support
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/assets/test.js", nil)
	req.Header.Add(headerAcceptEncoding, "gzip")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	t.Logf("Response Status: %d", w.Code)
	t.Logf("Content-Encoding: %s", w.Header().Get(headerContentEncoding))
	t.Logf("Vary: %s", w.Header().Get(headerVary))
	t.Logf("Content-Length: %s", w.Header().Get("Content-Length"))
	t.Logf("Body Length: %d", w.Body.Len())

	// This test will currently fail due to the bug described in issue #122
	// The static handler sets status to 404 initially, causing gzip middleware to remove headers
	assert.Equal(t, http.StatusOK, w.Code)

	// These assertions will fail with the current bug:
	// - Content-Encoding header will be empty instead of "gzip"
	// - Vary header will be empty instead of "Accept-Encoding"
	// - Content will not be compressed
	if w.Header().Get(headerContentEncoding) != "gzip" {
		t.Errorf("BUG REPRODUCED: Static file is not being gzip compressed. Content-Encoding: %q, expected: %q",
			w.Header().Get(headerContentEncoding), "gzip")
	}

	if w.Header().Get(headerVary) != headerAcceptEncoding {
		t.Errorf("BUG REPRODUCED: Vary header missing. Vary: %q, expected: %q",
			w.Header().Get(headerVary), headerAcceptEncoding)
	}
}