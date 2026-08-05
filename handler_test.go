package gzip

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// ClosedReader wraps an io.Reader to read data by chunks
// and returns an error if the reader is prematurely closed.
type ClosedReader struct {
	reader    io.Reader
	chunkSize int
	closed    bool
}

func NewClosedReader(reader io.Reader, chunkSize int) io.Reader {
	return &ClosedReader{
		reader:    reader,
		chunkSize: chunkSize,
	}
}

func (r *ClosedReader) Read(b []byte) (n int, err error) {
	if r.closed {
		return 0, io.EOF
	}

	return r.reader.Read(b[0 : min(r.chunkSize, len(b))-1])
}

func (r *ClosedReader) Close() (err error) {
	r.closed = true
	if closer, ok := r.reader.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

func TestHandleGzip(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name                    string
		path                    string
		acceptEncoding          string
		expectedContentEncoding string
		expectedBody            string
	}{
		{
			name:                    "Gzip compression",
			path:                    "/",
			acceptEncoding:          gzipEncoding,
			expectedContentEncoding: gzipEncoding,
			expectedBody:            "Gzip Test Response",
		},
		{
			name:                    "No compression",
			path:                    "/",
			acceptEncoding:          "",
			expectedContentEncoding: "",
			expectedBody:            "Gzip Test Response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(Gzip(DefaultCompression))
			router.GET("/", func(c *gin.Context) {
				c.String(http.StatusOK, "Gzip Test Response")
			})

			req, _ := http.NewRequestWithContext(context.Background(), "GET", tt.path, nil)
			req.Header.Set(headerAcceptEncoding, tt.acceptEncoding)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, tt.expectedContentEncoding, w.Header().Get("Content-Encoding"))

			if tt.expectedContentEncoding == gzipEncoding {
				gr, err := gzip.NewReader(w.Body)
				assert.NoError(t, err)
				defer gr.Close()

				body, _ := io.ReadAll(gr)
				assert.Equal(t, tt.expectedBody, string(body))
			} else {
				assert.Equal(t, tt.expectedBody, w.Body.String())
			}
		})
	}
}

func TestHandleDecompressGzip(t *testing.T) {
	gin.SetMode(gin.TestMode)

	buf := &bytes.Buffer{}
	gz, _ := gzip.NewWriterLevel(buf, gzip.DefaultCompression)
	if _, err := gz.Write([]byte("Gzip Test Response")); err != nil {
		gz.Close()
		t.Fatal(err)
	}
	gz.Close()

	router := gin.New()
	router.Use(Gzip(DefaultCompression, WithDecompressFn(DefaultDecompressHandle)))
	router.POST("/", func(c *gin.Context) {
		data, err := c.GetRawData()
		assert.NoError(t, err)
		assert.Equal(t, "Gzip Test Response", string(data))
		c.String(http.StatusOK, "ok")
	})

	req, _ := http.NewRequestWithContext(context.Background(), "POST", "/", buf)
	req.Header.Set("Content-Encoding", gzipEncoding)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "", w.Header().Get("Content-Encoding"))
	assert.Equal(t, "ok", w.Body.String())
}

func TestHandleDecompressGzipCompressResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name                    string
		acceptEncoding          string
		gzipIterations          uint
		expectedContentEncoding string
		expectedBody            string
	}{
		{
			name:                    "decompress with gzip accept-encoding should compress response",
			acceptEncoding:          gzipEncoding,
			gzipIterations:          1,
			expectedContentEncoding: gzipEncoding,
			expectedBody:            "Gzip Test Response",
		},
		{
			name:                    "decompress without gzip accept-encoding should not compress response",
			acceptEncoding:          "",
			gzipIterations:          1,
			expectedContentEncoding: "",
			expectedBody:            "Gzip Test Response",
		},
		{
			name:                    "decompress multi compressed request with gzip accept-encoding should compress response",
			acceptEncoding:          gzipEncoding,
			gzipIterations:          3,
			expectedContentEncoding: gzipEncoding,
			expectedBody:            "Gzip Test Response",
		},
		{
			name:                    "decompress multi compressed request without gzip accept-encoding should not compress response",
			acceptEncoding:          "",
			gzipIterations:          3,
			expectedContentEncoding: "",
			expectedBody:            "Gzip Test Response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.Default()
			router.Use(Gzip(DefaultCompression, WithDecompressFn(DefaultDecompressHandle)))
			router.POST("/", func(c *gin.Context) {
				data, err := c.GetRawData()
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedBody, string(data))
				c.String(http.StatusOK, tt.expectedBody)
			})

			data := []byte(tt.expectedBody)
			headers := []string{}
			for range tt.gzipIterations {
				headers = append(headers, gzipEncoding)
				buf := &bytes.Buffer{}
				gz, _ := gzip.NewWriterLevel(buf, gzip.DefaultCompression)
				if _, err := gz.Write(data); err != nil {
					gz.Close()
					t.Fatal(err)
				}
				gz.Close()
				data = buf.Bytes()
			}

			// Simulate a buffering network request
			// It permits to illustrate the behaviour
			// when DefaultDecompressHandle closes an half read request
			reader := NewClosedReader(bytes.NewReader(data), len(data)/2)

			req, _ := http.NewRequestWithContext(context.Background(), "POST", "/", reader)
			req.Header.Add("Content-Encoding", strings.Join(headers, ", "))
			if tt.acceptEncoding != "" {
				req.Header.Set(headerAcceptEncoding, tt.acceptEncoding)
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, tt.expectedContentEncoding, w.Header().Get("Content-Encoding"))

			if tt.expectedContentEncoding == gzipEncoding {
				gr, err := gzip.NewReader(w.Body)
				assert.NoError(t, err)
				defer gr.Close()

				body, _ := io.ReadAll(gr)
				assert.Equal(t, tt.expectedBody, string(body))
			} else {
				assert.Equal(t, tt.expectedBody, w.Body.String())
			}
		})
	}
}

func TestHandle404NoCompression(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		acceptEncoding string
		expectedGzip   bool
	}{
		{
			name:           "404 with gzip accept-encoding should not compress",
			acceptEncoding: gzipEncoding,
			expectedGzip:   false,
		},
		{
			name:           "404 without gzip accept-encoding",
			acceptEncoding: "",
			expectedGzip:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(Gzip(DefaultCompression))
			// Register a route to get proper 404 for unmatched paths
			router.NoRoute(func(c *gin.Context) {
				c.String(http.StatusNotFound, "404 page not found")
			})

			req, _ := http.NewRequestWithContext(context.Background(), "GET", "/nonexistent", nil)
			if tt.acceptEncoding != "" {
				req.Header.Set(headerAcceptEncoding, tt.acceptEncoding)
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusNotFound, w.Code)

			// Check that Content-Encoding header is not set for 404 responses
			contentEncoding := w.Header().Get("Content-Encoding")
			if tt.expectedGzip {
				assert.Equal(t, gzipEncoding, contentEncoding)
			} else {
				assert.Empty(t, contentEncoding, "404 responses should not have Content-Encoding: gzip")
			}

			// Verify that Vary header is also not set for uncompressed 404 responses
			if !tt.expectedGzip {
				vary := w.Header().Get("Vary")
				assert.Empty(t, vary, "404 responses should not have Vary header when not compressed")
			}
		})
	}
}
