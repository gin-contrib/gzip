package gzipfork

import (
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/elephant-insurance/go-microservice-arch/v3/clicker"
	"github.com/gin-gonic/gin"
)

var (
	// CompressedBytes counts the bytes of compressed requests before decompression
	CompressedBytes = &clicker.Clicker{}
	// DecompressedBytes counts the bytes of compressed requests after decompression
	DecompressedBytes = &clicker.Clicker{}
	// UncompressedBytes counts the bytes of uncompressed requests
	UncompressedBytes    = &clicker.Clicker{}
	CompressedRequests   = &clicker.Clicker{}
	UncompressedRequests = &clicker.Clicker{}
)

type gzipHandler struct {
	*Options
	gzPool sync.Pool
}

func newGzipHandler(level int, options ...Option) *gzipHandler {
	handler := &gzipHandler{
		Options: DefaultOptions,
		gzPool: sync.Pool{
			New: func() interface{} {
				gz, err := gzip.NewWriterLevel(ioutil.Discard, level)
				if err != nil {
					panic(err)
				}
				return gz
			},
		},
	}
	for _, setter := range options {
		setter(handler.Options)
	}
	return handler
}

func (g *gzipHandler) Handle(c *gin.Context) {
	if fn := g.DecompressFn; fn != nil && g.shouldDecompress(c) {
		before, after := fn(c)
		CompressedRequests.Click(1)
		CompressedBytes.Click(before)
		DecompressedBytes.Click(after)
	} else {
		cls := c.Request.Header.Get("Content-Length")
		if bl, _ := strconv.Atoi(cls); bl > 0 {
			UncompressedRequests.Click(1)
			UncompressedBytes.Click(bl)
		}
		return
	}

	gz := g.gzPool.Get().(*gzip.Writer)
	defer g.gzPool.Put(gz)
	defer gz.Reset(ioutil.Discard)
	gz.Reset(c.Writer)

	c.Header("Content-Encoding", "gzip")
	c.Header("Vary", "Accept-Encoding")
	c.Writer = &gzipWriter{c.Writer, gz}
	defer func() {
		gz.Close()
		c.Header("Content-Length", fmt.Sprint(c.Writer.Size()))
	}()
	c.Next()
}

func (g *gzipHandler) shouldCompress(req *http.Request) bool {
	if !strings.Contains(req.Header.Get("Accept-Encoding"), "gzip") ||
		strings.Contains(req.Header.Get("Connection"), "Upgrade") ||
		strings.Contains(req.Header.Get("Accept"), "text/event-stream") {
		return false
	}

	extension := filepath.Ext(req.URL.Path)
	if g.ExcludedExtensions.Contains(extension) {
		return false
	}

	if g.ExcludedPaths.Contains(req.URL.Path) {
		return false
	}
	if g.ExcludedPathesRegexs.Contains(req.URL.Path) {
		return false
	}

	return true
}

// shouldDecompress returns true if the Content-Encoding is "gzip" or "application/gzip"
// TODO: detect bad header for non-compressed request body and return false for fault tolerance
func (g *gzipHandler) shouldDecompress(c *gin.Context) bool {
	const (
		encHeaderKey    = `Content-Encoding`
		encHeaderZipVal = `gzip`
	)
	if c != nil && c.Request != nil {
		enc := strings.ToLower(c.Request.Header.Get(encHeaderKey))
		if strings.HasSuffix(enc, encHeaderZipVal) {
			return true
		}
	}

	return false
}
