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
		c.Header("Content-Length", strconv.Itoa(len(testResponse)))
		c.String(200, testResponse)
	})
	router.GET("/ping", func(c *gin.Context) {
		c.Writer.Header().Add("Vary", "Origin")
	}, func(c *gin.Context) {
		c.Header("Content-Length", strconv.Itoa(len(testResponse)))
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
	assert.NotEqual(t, "0", w.Header().Get("Content-Length"))
	assert.NotEqual(t, 19, w.Body.Len())
	assert.Equal(t, w.Header().Get("Content-Length"), fmt.Sprint(w.Body.Len()))

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
	assert.NotEqual(t, w.Header().Get("Content-Length"), "0")
	assert.NotEqual(t, w.Body.Len(), 19)
	assert.Equal(t, fmt.Sprint(w.Body.Len()), w.Header().Get("Content-Length"))

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
		assert.Equal(t, tt.expectedContentLength, w.Header().Get("Content-Length"))
	}
}

func TestNoGzip(t *testing.T) {
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/", nil)

	w := httptest.NewRecorder()
	r := newServer()
	r.ServeHTTP(w, req)

	assert.Equal(t, w.Code, 200)
	assert.Equal(t, w.Header().Get(headerContentEncoding), "")
	assert.Equal(t, w.Header().Get("Content-Length"), "19")
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
	assert.NotEqual(t, w.Header().Get("Content-Length"), "0")
	assert.NotEqual(t, w.Body.Len(), 19)
	assert.Equal(t, fmt.Sprint(w.Body.Len()), w.Header().Get("Content-Length"))

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
		if v := c.Request.Header.Get("Content-Length"); v != "" {
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
	assert.Equal(t, "", w.Header().Get("Content-Length"))
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
	assert.Equal(t, "", w.Header().Get("Content-Length"))
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
		if v := c.Request.Header.Get("Content-Length"); v != "" {
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
	assert.Equal(t, "", w.Header().Get("Content-Length"))
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
		assert.Equal(t, c.Request.Header.Get("Content-Length"), "")
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
		c.Header("Content-Length", strconv.Itoa(len(testResponse)))
		c.String(200, testResponse)
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "", w.Header().Get(headerContentEncoding))
	assert.Equal(t, "19", w.Header().Get("Content-Length"))
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
		c.Header("Content-Length", strconv.Itoa(len(testResponse)))
		c.String(200, testResponse)
	})

	hijackable := newHijackableResponse()
	router.ServeHTTP(hijackable, req)
	assert.True(t, hijackable.Hijacked)
}
