package gzip

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

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
			acceptEncoding:          "gzip",
			expectedContentEncoding: "gzip",
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

			if tt.expectedContentEncoding == "gzip" {
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
	req.Header.Set("Content-Encoding", "gzip")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "ok", w.Body.String())
}
