package gzip

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

const (
	BestCompression    = gzip.BestCompression
	BestSpeed          = gzip.BestSpeed
	DefaultCompression = gzip.DefaultCompression
	NoCompression      = gzip.NoCompression
)

func Gzip(level, minLength int) gin.HandlerFunc {
	var gzPool sync.Pool
	gzPool.New = func() interface{} {
		gz, err := gzip.NewWriterLevel(ioutil.Discard, level)
		if err != nil {
			panic(err)
		}
		return gz
	}
	return func(c *gin.Context) {
		if !shouldCompress(c.Request) {
			return
		}

		gz := gzPool.Get().(*gzip.Writer)
		defer gzPool.Put(gz)
		gz.Reset(c.Writer)

		gzWriter := &gzipWriter{
			ResponseWriter: c.Writer,
			writer:         gz,
			minLength:      minLength,
		}

		// Replace the context writer with a gzip writer
		c.Writer = gzWriter

		c.Next()

		if gzWriter.compress {
			// Just close and flush the gz writer
			gz.Close()
		} else {
			// Discard the gz writer
			gz.Reset(ioutil.Discard)

			// Write the buffered data into the original writer
			gzWriter.ResponseWriter.Write(gzWriter.buffer.Bytes())
		}

		// Set the content length if it's still possible
		c.Header("Content-Length", fmt.Sprint(c.Writer.Size()))
	}
}

type gzipWriter struct {
	gin.ResponseWriter
	writer *gzip.Writer

	buffer    bytes.Buffer
	minLength int
	compress  bool
}

func (g *gzipWriter) WriteString(s string) (int, error) {
	return g.Write([]byte(s))
}

func (g *gzipWriter) Write(data []byte) (w int, err error) {
	// If the first chunk of data is already bigger than the minimum size,
	// set the headers and write directly to the gz writer
	if g.compress == false && len(data) >= g.minLength {
		g.ResponseWriter.Header().Set("Content-Encoding", "gzip")
		g.ResponseWriter.Header().Set("Vary", "Accept-Encoding")

		g.compress = true
	}

	if !g.compress {
		// Write the data into a buffer
		w, err = g.buffer.Write(data)
		if err != nil {
			return
		}

		// If the buffer is bigger than the minimum size, set the headers and write
		// the buffered data into the gz writer
		if g.buffer.Len() >= g.minLength {
			g.ResponseWriter.Header().Set("Content-Encoding", "gzip")
			g.ResponseWriter.Header().Set("Vary", "Accept-Encoding")

			_, err = g.writer.Write(g.buffer.Bytes())
			g.compress = true
		}

		return
	}

	// Write the data into the gz writer
	w, err = g.writer.Write(data)

	return
}

// Fix: https://github.com/mholt/caddy/issues/38
func (g *gzipWriter) WriteHeader(code int) {
	g.Header().Del("Content-Length")
	g.ResponseWriter.WriteHeader(code)
}

func shouldCompress(req *http.Request) bool {
	if !strings.Contains(req.Header.Get("Accept-Encoding"), "gzip") ||
		strings.Contains(req.Header.Get("Connection"), "Upgrade") ||
	        strings.Contains(req.Header.Get("Content-Type"), "text/event-stream") {

		return false
	}

	extension := filepath.Ext(req.URL.Path)
	if len(extension) < 4 { // fast path
		return true
	}

	switch extension {
	case ".png", ".gif", ".jpeg", ".jpg":
		return false
	default:
		return true
	}
}
