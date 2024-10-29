package gzip

import (
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
)

var (
	DefaultExcludedExtentions = NewExcludedExtensions([]string{
		".png", ".gif", ".jpeg", ".jpg",
	})
	DefaultOptions = &Options{
		ExcludedExtensions: DefaultExcludedExtentions,
	}
	UnsupportedContentEncoding = errors.New("Unsupported content encoding")
)

type Options struct {
	ExcludedExtensions   ExcludedExtensions
	ExcludedPaths        ExcludedPaths
	ExcludedPathesRegexs ExcludedPathesRegexs
	DecompressFn         func(c *gin.Context)
}

type Option func(*Options)

func WithExcludedExtensions(args []string) Option {
	return func(o *Options) {
		o.ExcludedExtensions = NewExcludedExtensions(args)
	}
}

func WithExcludedPaths(args []string) Option {
	return func(o *Options) {
		o.ExcludedPaths = NewExcludedPaths(args)
	}
}

func WithExcludedPathsRegexs(args []string) Option {
	return func(o *Options) {
		o.ExcludedPathesRegexs = NewExcludedPathesRegexs(args)
	}
}

func WithDecompressFn(decompressFn func(c *gin.Context)) Option {
	return func(o *Options) {
		o.DecompressFn = decompressFn
	}
}

// Using map for better lookup performance
type ExcludedExtensions map[string]bool

func NewExcludedExtensions(extensions []string) ExcludedExtensions {
	res := make(ExcludedExtensions)
	for _, e := range extensions {
		res[e] = true
	}
	return res
}

func (e ExcludedExtensions) Contains(target string) bool {
	_, ok := e[target]
	return ok
}

type ExcludedPaths []string

func NewExcludedPaths(paths []string) ExcludedPaths {
	return ExcludedPaths(paths)
}

func (e ExcludedPaths) Contains(requestURI string) bool {
	for _, path := range e {
		if strings.HasPrefix(requestURI, path) {
			return true
		}
	}
	return false
}

type ExcludedPathesRegexs []*regexp.Regexp

func NewExcludedPathesRegexs(regexs []string) ExcludedPathesRegexs {
	result := make([]*regexp.Regexp, len(regexs))
	for i, reg := range regexs {
		result[i] = regexp.MustCompile(reg)
	}
	return result
}

func (e ExcludedPathesRegexs) Contains(requestURI string) bool {
	for _, reg := range e {
		if reg.MatchString(requestURI) {
			return true
		}
	}
	return false
}

func DefaultDecompressHandle(c *gin.Context) {
	if c.Request.Body == nil {
		return
	}

	contentEncodingField := strings.Split(strings.ToLower(c.GetHeader("Content-Encoding")), ",")
	if len(contentEncodingField) == 0 { // nothing to decompress
		c.Next()

		return
	}

	toClose := make([]io.Closer, 0, len(contentEncodingField))
	defer func() {
		for i := len(toClose); i > 0; i-- {
			toClose[i-1].Close()
		}
	}()

	// parses multiply gzips like
	// Content-Encoding: gzip, gzip, gzip
	// allowed by RFC
	for i := 0; i < len(contentEncodingField); i++ {
		trimmedValue := strings.TrimSpace(contentEncodingField[i])

		if trimmedValue == "" {
			continue
		}

		if trimmedValue != "gzip" {
			// https://www.rfc-editor.org/rfc/rfc7231#section-3.1.2.2
			// An origin server MAY respond with a status code of 415 (Unsupported
			// Media Type) if a representation in the request message has a content
			// coding that is not acceptable.
			_ = c.AbortWithError(http.StatusUnsupportedMediaType, UnsupportedContentEncoding)
		}

		r, err := gzip.NewReader(c.Request.Body)
		if err != nil {
			_ = c.AbortWithError(http.StatusBadRequest, err)

			return
		}

		toClose = append(toClose, c.Request.Body)

		c.Request.Body = r
	}

	c.Request.Header.Del("Content-Encoding")
	c.Request.Header.Del("Content-Length")

	c.Next()
}
