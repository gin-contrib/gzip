package gzip

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testResponse        = "Gzip Test Response "
	testReverseResponse = "Gzip Test Reverse Response "
)

type rServer struct{}

func (s *rServer) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	fmt.Fprint(rw, testReverseResponse)
}

type closeNotifyingRecorder struct {
	*httptest.ResponseRecorder
	closed chan bool
}

func newCloseNotifyingRecorder() *closeNotifyingRecorder {
	return &closeNotifyingRecorder{
		httptest.NewRecorder(),
		make(chan bool, 1),
	}
}

func (c *closeNotifyingRecorder) CloseNotify() <-chan bool {
	return c.closed
}

func newServer() *gin.Engine {
	// init reverse proxy server
	rServer := httptest.NewServer(new(rServer))
	target, _ := url.Parse(rServer.URL)
	rp := httputil.NewSingleHostReverseProxy(target)

	router := gin.New()
	router.Use(Gzip(DefaultCompression))
	router.GET("/", func(c *gin.Context) {
		c.Header(headerContentLength, strconv.Itoa(len(testResponse)))
		c.String(200, testResponse)
	})
	router.GET("/ping", func(c *gin.Context) {
		c.Writer.Header().Add(headerVary, "Origin")
	}, func(c *gin.Context) {
		c.Header(headerContentLength, strconv.Itoa(len(testResponse)))
		c.String(200, testResponse)
	})
	router.Any("/reverse", func(c *gin.Context) {
		rp.ServeHTTP(c.Writer, c.Request)
	})
	return router
}

func TestVaryHeader(t *testing.T) {
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/ping", nil)
	req.Header.Add(headerAcceptEncoding, "gzip")

	w := httptest.NewRecorder()
	r := newServer()
	r.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "gzip", w.Header().Get(headerContentEncoding))
	assert.Equal(t, []string{headerAcceptEncoding, "Origin"}, w.Header().Values(headerVary))
	assert.NotEqual(t, "0", w.Header().Get(headerContentLength))
	assert.NotEqual(t, 19, w.Body.Len())
	assert.Equal(t, w.Header().Get(headerContentLength), fmt.Sprint(w.Body.Len()))

	gr, err := gzip.NewReader(w.Body)
	assert.NoError(t, err)
	defer gr.Close()

	body, _ := io.ReadAll(gr)
	assert.Equal(t, testResponse, string(body))
}

func TestGzip(t *testing.T) {
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/", nil)
	req.Header.Add(headerAcceptEncoding, "gzip")

	w := httptest.NewRecorder()
	r := newServer()
	r.ServeHTTP(w, req)

	assert.Equal(t, w.Code, 200)
	assert.Equal(t, w.Header().Get(headerContentEncoding), "gzip")
	assert.Equal(t, w.Header().Get(headerVary), headerAcceptEncoding)
	assert.NotEqual(t, w.Header().Get(headerContentLength), "0")
	assert.NotEqual(t, w.Body.Len(), 19)
	assert.Equal(t, fmt.Sprint(w.Body.Len()), w.Header().Get(headerContentLength))

	gr, err := gzip.NewReader(w.Body)
	assert.NoError(t, err)
	defer gr.Close()

	body, _ := io.ReadAll(gr)
	assert.Equal(t, string(body), testResponse)
}

func TestGzipPNG(t *testing.T) {
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/image.png", nil)
	req.Header.Add(headerAcceptEncoding, "gzip")

	router := gin.New()
	router.Use(Gzip(DefaultCompression))
	router.GET("/image.png", func(c *gin.Context) {
		c.String(200, "this is a PNG!")
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, w.Code, 200)
	assert.Equal(t, w.Header().Get(headerContentEncoding), "")
	assert.Equal(t, w.Header().Get(headerVary), "")
	assert.Equal(t, w.Body.String(), "this is a PNG!")
}

func TestExcludedPathsAndExtensions(t *testing.T) {
	tests := []struct {
		path                    string
		option                  Option
		expectedContentEncoding string
		expectedVary            string
		expectedBody            string
		expectedContentLength   string
	}{
		{"/api/books", WithExcludedPaths([]string{"/api/"}), "", "", "this is books!", ""},
		{"/index.html", WithExcludedExtensions([]string{".html"}), "", "", "this is a HTML!", ""},
	}

	for _, tt := range tests {
		req, _ := http.NewRequestWithContext(context.Background(), "GET", tt.path, nil)
		req.Header.Add(headerAcceptEncoding, "gzip")

		router := gin.New()
		router.Use(Gzip(DefaultCompression, tt.option))
		router.GET(tt.path, func(c *gin.Context) {
			c.String(200, tt.expectedBody)
		})

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, tt.expectedContentEncoding, w.Header().Get(headerContentEncoding))
		assert.Equal(t, tt.expectedVary, w.Header().Get(headerVary))
		assert.Equal(t, tt.expectedBody, w.Body.String())
		assert.Equal(t, tt.expectedContentLength, w.Header().Get(headerContentLength))
	}
}

func TestNoGzip(t *testing.T) {
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/", nil)

	w := httptest.NewRecorder()
	r := newServer()
	r.ServeHTTP(w, req)

	assert.Equal(t, w.Code, 200)
	assert.Equal(t, w.Header().Get(headerContentEncoding), "")
	assert.Equal(t, w.Header().Get(headerContentLength), "19")
	assert.Equal(t, w.Body.String(), testResponse)
}

func TestGzipWithReverseProxy(t *testing.T) {
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/reverse", nil)
	req.Header.Add(headerAcceptEncoding, "gzip")

	w := newCloseNotifyingRecorder()
	r := newServer()
	r.ServeHTTP(w, req)

	assert.Equal(t, w.Code, 200)
	assert.Equal(t, w.Header().Get(headerContentEncoding), "gzip")
	assert.Equal(t, w.Header().Get(headerVary), headerAcceptEncoding)
	assert.NotEqual(t, w.Header().Get(headerContentLength), "0")
	assert.NotEqual(t, w.Body.Len(), 19)
	assert.Equal(t, fmt.Sprint(w.Body.Len()), w.Header().Get(headerContentLength))

	gr, err := gzip.NewReader(w.Body)
	assert.NoError(t, err)
	defer gr.Close()

	body, _ := io.ReadAll(gr)
	assert.Equal(t, string(body), testReverseResponse)
}

func TestDecompressGzip(t *testing.T) {
	buf := &bytes.Buffer{}
	gz, _ := gzip.NewWriterLevel(buf, gzip.DefaultCompression)
	if _, err := gz.Write([]byte(testResponse)); err != nil {
		gz.Close()
		t.Fatal(err)
	}
	gz.Close()

	req, _ := http.NewRequestWithContext(context.Background(), "POST", "/", buf)
	req.Header.Add(headerContentEncoding, "gzip")

	router := gin.New()
	router.Use(Gzip(DefaultCompression, WithDecompressFn(DefaultDecompressHandle)))
	router.POST("/", func(c *gin.Context) {
		if v := c.Request.Header.Get(headerContentEncoding); v != "" {
			t.Errorf("unexpected `Content-Encoding`: %s header", v)
		}
		if v := c.Request.Header.Get(headerContentLength); v != "" {
			t.Errorf("unexpected `Content-Length`: %s header", v)
		}
		data, err := c.GetRawData()
		if err != nil {
			t.Fatal(err)
		}
		c.Data(200, "text/plain", data)
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "", w.Header().Get(headerContentEncoding))
	assert.Equal(t, "", w.Header().Get(headerVary))
	assert.Equal(t, testResponse, w.Body.String())
	assert.Equal(t, "", w.Header().Get(headerContentLength))
}

func TestDecompressGzipWithEmptyBody(t *testing.T) {
	req, _ := http.NewRequestWithContext(context.Background(), "POST", "/", nil)
	req.Header.Add(headerContentEncoding, "gzip")

	router := gin.New()
	router.Use(Gzip(DefaultCompression, WithDecompressFn(DefaultDecompressHandle)))
	router.POST("/", func(c *gin.Context) {
		c.String(200, "ok")
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "", w.Header().Get(headerContentEncoding))
	assert.Equal(t, "", w.Header().Get(headerVary))
	assert.Equal(t, "ok", w.Body.String())
	assert.Equal(t, "", w.Header().Get(headerContentLength))
}

func TestDecompressGzipWithIncorrectData(t *testing.T) {
	req, _ := http.NewRequestWithContext(context.Background(), "POST", "/", bytes.NewReader([]byte(testResponse)))
	req.Header.Add(headerContentEncoding, "gzip")

	router := gin.New()
	router.Use(Gzip(DefaultCompression, WithDecompressFn(DefaultDecompressHandle)))
	router.POST("/", func(c *gin.Context) {
		c.String(200, "ok")
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDecompressOnly(t *testing.T) {
	buf := &bytes.Buffer{}
	gz, _ := gzip.NewWriterLevel(buf, gzip.DefaultCompression)
	if _, err := gz.Write([]byte(testResponse)); err != nil {
		gz.Close()
		t.Fatal(err)
	}
	gz.Close()

	req, _ := http.NewRequestWithContext(context.Background(), "POST", "/", buf)
	req.Header.Add(headerContentEncoding, "gzip")

	router := gin.New()
	router.Use(Gzip(NoCompression, WithDecompressOnly(), WithDecompressFn(DefaultDecompressHandle)))
	router.POST("/", func(c *gin.Context) {
		if v := c.Request.Header.Get(headerContentEncoding); v != "" {
			t.Errorf("unexpected `Content-Encoding`: %s header", v)
		}
		if v := c.Request.Header.Get(headerContentLength); v != "" {
			t.Errorf("unexpected `Content-Length`: %s header", v)
		}
		data, err := c.GetRawData()
		if err != nil {
			t.Fatal(err)
		}
		c.Data(200, "text/plain", data)
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "", w.Header().Get(headerContentEncoding))
	assert.Equal(t, "", w.Header().Get(headerVary))
	assert.Equal(t, testResponse, w.Body.String())
	assert.Equal(t, "", w.Header().Get(headerContentLength))
}

func TestGzipWithDecompressOnly(t *testing.T) {
	buf := &bytes.Buffer{}
	gz, _ := gzip.NewWriterLevel(buf, gzip.DefaultCompression)
	if _, err := gz.Write([]byte(testResponse)); err != nil {
		gz.Close()
		t.Fatal(err)
	}
	gz.Close()

	req, _ := http.NewRequestWithContext(context.Background(), "POST", "/", buf)
	req.Header.Add(headerContentEncoding, "gzip")
	req.Header.Add(headerAcceptEncoding, "gzip")

	r := gin.New()
	r.Use(Gzip(NoCompression, WithDecompressOnly(), WithDecompressFn(DefaultDecompressHandle)))
	r.POST("/", func(c *gin.Context) {
		assert.Equal(t, c.Request.Header.Get(headerContentEncoding), "")
		assert.Equal(t, c.Request.Header.Get(headerContentLength), "")
		body, err := c.GetRawData()
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, testResponse, string(body))
		c.String(200, testResponse)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "", w.Header().Get(headerContentEncoding))
	assert.Equal(t, testResponse, w.Body.String())
}

func TestCustomShouldCompressFn(t *testing.T) {
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/", nil)
	req.Header.Add(headerAcceptEncoding, "gzip")

	router := gin.New()
	router.Use(Gzip(
		DefaultCompression,
		WithCustomShouldCompressFn(func(_ *gin.Context) bool {
			return false
		}),
	))
	router.GET("/", func(c *gin.Context) {
		c.Header(headerContentLength, strconv.Itoa(len(testResponse)))
		c.String(200, testResponse)
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "", w.Header().Get(headerContentEncoding))
	assert.Equal(t, "19", w.Header().Get(headerContentLength))
	assert.Equal(t, testResponse, w.Body.String())
}

type hijackableResponse struct {
	Hijacked bool
	header   http.Header
}

func newHijackableResponse() *hijackableResponse {
	return &hijackableResponse{header: make(http.Header)}
}

func (h *hijackableResponse) Header() http.Header       { return h.header }
func (h *hijackableResponse) Write([]byte) (int, error) { return 0, nil }
func (h *hijackableResponse) WriteHeader(int)           {}
func (h *hijackableResponse) Flush()                    {}
func (h *hijackableResponse) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h.Hijacked = true
	return nil, nil, nil
}

func TestResponseWriterHijack(t *testing.T) {
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/", nil)
	req.Header.Add(headerAcceptEncoding, "gzip")

	router := gin.New()
	router.Use(Gzip(
		DefaultCompression,
		WithCustomShouldCompressFn(func(_ *gin.Context) bool {
			return false
		}),
	)).Use(gin.HandlerFunc(func(c *gin.Context) {
		hj, ok := c.Writer.(http.Hijacker)
		require.True(t, ok)

		_, _, err := hj.Hijack()
		assert.Nil(t, err)
		c.Next()
	}))
	router.GET("/", func(c *gin.Context) {
		c.Header(headerContentLength, strconv.Itoa(len(testResponse)))
		c.String(200, testResponse)
	})

	hijackable := newHijackableResponse()
	router.ServeHTTP(hijackable, req)
	assert.True(t, hijackable.Hijacked)
}

func TestDoubleGzipCompression(t *testing.T) {
	// Create a test server that returns gzip-compressed content
	compressedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Compress the response body
		buf := &bytes.Buffer{}
		gz := gzip.NewWriter(buf)
		_, err := gz.Write([]byte(testReverseResponse))
		require.NoError(t, err)
		require.NoError(t, gz.Close())

		// Set gzip headers to simulate already compressed content
		w.Header().Set(headerContentEncoding, "gzip")
		w.Header().Set(headerContentLength, strconv.Itoa(buf.Len()))
		w.WriteHeader(200)
		_, err = w.Write(buf.Bytes())
		require.NoError(t, err)
	}))
	defer compressedServer.Close()

	// Parse the server URL for the reverse proxy
	target, err := url.Parse(compressedServer.URL)
	require.NoError(t, err)
	rp := httputil.NewSingleHostReverseProxy(target)

	// Create gin router with gzip middleware
	router := gin.New()
	router.Use(Gzip(DefaultCompression))
	router.Any("/proxy", func(c *gin.Context) {
		rp.ServeHTTP(c.Writer, c.Request)
	})

	// Make request through the proxy
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/proxy", nil)
	req.Header.Add(headerAcceptEncoding, "gzip")

	w := newCloseNotifyingRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	// Check if response is compressed - should still be gzip since upstream provided gzip
	// But it should NOT be double compressed
	responseBody := w.Body.Bytes()

	// First check if the response body looks like gzip (starts with gzip magic number)
	if len(responseBody) >= 2 && responseBody[0] == 0x1f && responseBody[1] == 0x8b {
		// Response is gzip compressed, try to decompress once
		gr, err := gzip.NewReader(bytes.NewReader(responseBody))
		assert.NoError(t, err, "Response should be decompressible with single gzip decompression")
		defer gr.Close()

		body, err := io.ReadAll(gr)
		assert.NoError(t, err)
		assert.Equal(t, testReverseResponse, string(body),
			"Response should match original content after single decompression")

		// Ensure no double compression - decompressed content should not be gzip
		if len(body) >= 2 && body[0] == 0x1f && body[1] == 0x8b {
			t.Error("Response appears to be double-compressed - " +
				"single decompression revealed another gzip stream")
		}
	} else {
		// Response is not gzip compressed, check if content matches
		assert.Equal(t, testReverseResponse, w.Body.String(), "Uncompressed response should match original content")
	}
}

func TestPrometheusMetricsDoubleCompression(t *testing.T) {
	// Simulate Prometheus metrics server that returns gzip-compressed metrics
	prometheusData := `# HELP http_requests_total Total number of HTTP requests
# TYPE http_requests_total counter
http_requests_total{method="get",status="200"} 1027 1395066363000
http_requests_total{method="get",status="400"} 3 1395066363000`

	prometheusServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prometheus server compresses its own response
		buf := &bytes.Buffer{}
		gz := gzip.NewWriter(buf)
		_, err := gz.Write([]byte(prometheusData))
		require.NoError(t, err)
		require.NoError(t, gz.Close())

		w.Header().Set(headerContentEncoding, "gzip")
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.Header().Set(headerContentLength, strconv.Itoa(buf.Len()))
		w.WriteHeader(200)
		_, err = w.Write(buf.Bytes())
		require.NoError(t, err)
	}))
	defer prometheusServer.Close()

	// Create reverse proxy to Prometheus server
	target, err := url.Parse(prometheusServer.URL)
	require.NoError(t, err)
	rp := httputil.NewSingleHostReverseProxy(target)

	// Create gin router with gzip middleware (like what would happen in a gateway)
	router := gin.New()
	router.Use(Gzip(DefaultCompression))
	router.Any("/metrics", func(c *gin.Context) {
		rp.ServeHTTP(c.Writer, c.Request)
	})

	// Simulate Prometheus scraper request
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/metrics", nil)
	req.Header.Add(headerAcceptEncoding, "gzip")
	req.Header.Add("User-Agent", "Prometheus/2.37.0")

	w := newCloseNotifyingRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	// Check if response is gzip compressed and handle accordingly
	responseBody := w.Body.Bytes()

	// First check if the response body looks like gzip (starts with gzip magic number)
	if len(responseBody) >= 2 && responseBody[0] == 0x1f && responseBody[1] == 0x8b {
		// Response is gzip compressed, try to decompress once
		gr, err := gzip.NewReader(bytes.NewReader(responseBody))
		assert.NoError(t, err, "Prometheus should be able to decompress the metrics response")
		defer gr.Close()

		body, err := io.ReadAll(gr)
		assert.NoError(t, err)
		assert.Equal(t, prometheusData, string(body), "Metrics content should be correct after decompression")

		// Verify no double compression - decompressed content should not be gzip
		if len(body) >= 2 && body[0] == 0x1f && body[1] == 0x8b {
			t.Error("Metrics response appears to be double-compressed - Prometheus scraping would fail")
		}
	} else {
		// Response is not gzip compressed, check if content matches
		assert.Equal(t, prometheusData, w.Body.String(), "Uncompressed metrics should match original content")
	}
}
