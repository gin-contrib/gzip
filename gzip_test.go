package gzip

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

const (
	testResponse        = "Gzip Test Response "
	testReverseResponse = "Gzip Test Reverse Response "
)

func init() {
	gin.SetMode(gin.ReleaseMode)
}

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

func (c *closeNotifyingRecorder) close() {
	c.closed <- true
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
	router.Any("/reverse", func(c *gin.Context) {
		rp.ServeHTTP(c.Writer, c.Request)
	})
	return router
}

func TestGzip(t *testing.T) {
	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Add("Accept-Encoding", "gzip")

	w := httptest.NewRecorder()
	r := newServer()
	r.ServeHTTP(w, req)

	assert.Equal(t, w.Code, 200)
	assert.Equal(t, w.Header().Get("Content-Encoding"), "gzip")
	assert.Equal(t, w.Header().Get("Vary"), "Accept-Encoding")
	assert.NotEqual(t, w.Header().Get("Content-Length"), "0")
	assert.NotEqual(t, w.Body.Len(), 19)
	assert.Equal(t, fmt.Sprint(w.Body.Len()), w.Header().Get("Content-Length"))

	gr, err := gzip.NewReader(w.Body)
	assert.NoError(t, err)
	defer gr.Close()

	body, _ := ioutil.ReadAll(gr)
	assert.Equal(t, string(body), testResponse)
}

func TestGzipPNG(t *testing.T) {
	req, _ := http.NewRequest("GET", "/image.png", nil)
	req.Header.Add("Accept-Encoding", "gzip")

	router := gin.New()
	router.Use(Gzip(DefaultCompression))
	router.GET("/image.png", func(c *gin.Context) {
		c.String(200, "this is a PNG!")
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, w.Code, 200)
	assert.Equal(t, w.Header().Get("Content-Encoding"), "")
	assert.Equal(t, w.Header().Get("Vary"), "")
	assert.Equal(t, w.Body.String(), "this is a PNG!")
}

func TestExcludedExtensions(t *testing.T) {
	req, _ := http.NewRequest("GET", "/index.html", nil)
	req.Header.Add("Accept-Encoding", "gzip")

	router := gin.New()
	router.Use(Gzip(DefaultCompression, WithExcludedExtensions([]string{".html"})))
	router.GET("/index.html", func(c *gin.Context) {
		c.String(200, "this is a HTML!")
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "", w.Header().Get("Content-Encoding"))
	assert.Equal(t, "", w.Header().Get("Vary"))
	assert.Equal(t, "this is a HTML!", w.Body.String())
	assert.Equal(t, "", w.Header().Get("Content-Length"))
}

func TestExcludedPaths(t *testing.T) {
	req, _ := http.NewRequest("GET", "/api/books", nil)
	req.Header.Add("Accept-Encoding", "gzip")

	router := gin.New()
	router.Use(Gzip(DefaultCompression, WithExcludedPaths([]string{"/api/"})))
	router.GET("/api/books", func(c *gin.Context) {
		c.String(200, "this is books!")
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "", w.Header().Get("Content-Encoding"))
	assert.Equal(t, "", w.Header().Get("Vary"))
	assert.Equal(t, "this is books!", w.Body.String())
	assert.Equal(t, "", w.Header().Get("Content-Length"))
}

func TestNoGzip(t *testing.T) {
	req, _ := http.NewRequest("GET", "/", nil)

	w := httptest.NewRecorder()
	r := newServer()
	r.ServeHTTP(w, req)

	assert.Equal(t, w.Code, 200)
	assert.Equal(t, w.Header().Get("Content-Encoding"), "")
	assert.Equal(t, w.Header().Get("Content-Length"), "19")
	assert.Equal(t, w.Body.String(), testResponse)
}

func TestGzipWithReverseProxy(t *testing.T) {
	req, _ := http.NewRequest("GET", "/reverse", nil)
	req.Header.Add("Accept-Encoding", "gzip")

	w := newCloseNotifyingRecorder()
	r := newServer()
	r.ServeHTTP(w, req)

	assert.Equal(t, w.Code, 200)
	assert.Equal(t, w.Header().Get("Content-Encoding"), "gzip")
	assert.Equal(t, w.Header().Get("Vary"), "Accept-Encoding")
	assert.NotEqual(t, w.Header().Get("Content-Length"), "0")
	assert.NotEqual(t, w.Body.Len(), 19)
	assert.Equal(t, fmt.Sprint(w.Body.Len()), w.Header().Get("Content-Length"))

	gr, err := gzip.NewReader(w.Body)
	assert.NoError(t, err)
	defer gr.Close()

	body, _ := ioutil.ReadAll(gr)
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

	req, _ := http.NewRequest("POST", "/", buf)
	req.Header.Add("Content-Encoding", "gzip")

	router := gin.New()
	router.Use(Gzip(DefaultCompression, WithDecompressFn(DefaultDecompressHandle)))
	router.POST("/", func(c *gin.Context) {
		if v := c.Request.Header.Get("Content-Encoding"); v != "" {
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
	assert.Equal(t, "", w.Header().Get("Content-Encoding"))
	assert.Equal(t, "", w.Header().Get("Vary"))
	assert.Equal(t, testResponse, w.Body.String())
	assert.Equal(t, "", w.Header().Get("Content-Length"))
}

func TestDecompressGzipWithEmptyBody(t *testing.T) {
	req, _ := http.NewRequest("POST", "/", nil)
	req.Header.Add("Content-Encoding", "gzip")

	router := gin.New()
	router.Use(Gzip(DefaultCompression, WithDecompressFn(DefaultDecompressHandle)))
	router.POST("/", func(c *gin.Context) {
		c.String(200, "ok")
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "", w.Header().Get("Content-Encoding"))
	assert.Equal(t, "", w.Header().Get("Vary"))
	assert.Equal(t, "ok", w.Body.String())
	assert.Equal(t, "", w.Header().Get("Content-Length"))
}

func TestDecompressGzipWithIncorrectData(t *testing.T) {
	req, _ := http.NewRequest("POST", "/", bytes.NewReader([]byte(testResponse)))
	req.Header.Add("Content-Encoding", "gzip")

	router := gin.New()
	router.Use(Gzip(DefaultCompression, WithDecompressFn(DefaultDecompressHandle)))
	router.POST("/", func(c *gin.Context) {
		c.String(200, "ok")
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
