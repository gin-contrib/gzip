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

func (g *gzipHandler) Handle(c *gin.Context) {
	if fn := g.decompressFn; fn != nil && c.Request.Header.Get("Content-Encoding") == "gzip" {
		fn(c)
	}

	if g.decompressOnly || !g.shouldCompress(c.Request) {
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

	extension := filepath.Ext(req.URL.Path)
	if g.excludedExtensions.Contains(extension) {
		return false
	}

	if g.excludedPaths.Contains(req.URL.Path) {
		return false
	}
	if g.excludedPathesRegexs.Contains(req.URL.Path) {
		return false
	}

	return true
}
