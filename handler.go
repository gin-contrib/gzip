package gzip

import (
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

type gzipHandler struct {
	*config
	gzPool sync.Pool
}

func newGzipHandler(level int, opts ...Option) *gzipHandler {
	cfg := &config{
		excludedExtensions: DefaultExcludedExtentions,
	}

	// Apply each option to the config
	for _, o := range opts {
		o.apply(cfg)
	}

	handler := &gzipHandler{
		config: cfg,
		gzPool: sync.Pool{
			New: func() interface{} {
				gz, err := gzip.NewWriterLevel(io.Discard, level)
				if err != nil {
					panic(err)
				}
				return gz
			},
		},
	}
	return handler
}

// Handle is a middleware function for handling gzip compression in HTTP requests and responses.
// It first checks if the request has a "Content-Encoding" header set to "gzip" and if a decompression
// function is provided, it will call the decompression function. If the handler is set to decompress only,
// or if the custom compression decision function indicates not to compress, it will return early.
// Otherwise, it retrieves a gzip.Writer from the pool, sets the necessary response headers for gzip encoding,
// and wraps the response writer with a gzipWriter. After the request is processed, it ensures the gzip.Writer
// is properly closed and the "Content-Length" header is set based on the response size.
func (g *gzipHandler) Handle(c *gin.Context) {
	if fn := g.decompressFn; fn != nil && c.Request.Header.Get("Content-Encoding") == "gzip" {
		fn(c)
	}

	if g.decompressOnly ||
		(g.customShouldCompressFn != nil && !g.customShouldCompressFn(c)) ||
		(g.customShouldCompressFn == nil && !g.shouldCompress(c.Request)) {
		return
	}

	gz := g.gzPool.Get().(*gzip.Writer)
	defer g.gzPool.Put(gz)
	defer gz.Reset(io.Discard)
	gz.Reset(c.Writer)

	c.Header("Content-Encoding", "gzip")
	c.Header("Vary", "Accept-Encoding")
	c.Writer = &gzipWriter{c.Writer, gz}
	defer func() {
		if c.Writer.Size() < 0 {
			// do not write gzip footer when nothing is written to the response body
			gz.Reset(io.Discard)
		}
		gz.Close()
		c.Header("Content-Length", fmt.Sprint(c.Writer.Size()))
	}()
	c.Next()
}

func (g *gzipHandler) shouldCompress(req *http.Request) bool {
	if !strings.Contains(req.Header.Get("Accept-Encoding"), "gzip") ||
		strings.Contains(req.Header.Get("Connection"), "Upgrade") {
		return false
	}

	// Check if the request path is excluded from compression
	extension := filepath.Ext(req.URL.Path)
	if g.excludedExtensions.Contains(extension) ||
		g.excludedPaths.Contains(req.URL.Path) ||
		g.excludedPathesRegexs.Contains(req.URL.Path) {
		return false
	}

	return true
}
