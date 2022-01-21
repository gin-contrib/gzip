package gzip

import (
	"compress/gzip"

	"github.com/gin-gonic/gin"
)

const (
	BestCompression    = gzip.BestCompression
	BestSpeed          = gzip.BestSpeed
	DefaultCompression = gzip.DefaultCompression
	NoCompression      = gzip.NoCompression
)

func Gzip(level int, options ...Option) gin.HandlerFunc {
	return newGzipHandler(level, options...).Handle
}

type gzipWriter struct {
	gin.ResponseWriter
	writer  *gzip.Writer
	gzipped bool
}

func (g *gzipWriter) WriteString(s string) (int, error) {
	g.Header().Del("Content-Length")
	return g.writer.Write([]byte(s))
}

func (g *gzipWriter) Write(data []byte) (int, error) {
	g.gzipped = g.isGzipped(data) || g.gzipped
	if g.isGzipped(data) {
		return g.ResponseWriter.Write(data)
	}

	g.Header().Set("Content-Encoding", "gzip")
	g.Header().Set("Vary", "Accept-Encoding")
	g.Header().Del("Content-Length")
	return g.writer.Write(data)
}

// Fix: https://github.com/mholt/caddy/issues/38
func (g *gzipWriter) WriteHeader(code int) {
	g.Header().Del("Content-Length")
	g.ResponseWriter.WriteHeader(code)
}

func (g *gzipWriter) isGzipped(input []byte) bool {
	if len(input) < 2 {
		return false
	}
	return input[0] == 0x1f && input[1] == 0x8b
}
