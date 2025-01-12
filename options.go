package gzip

import (
	"compress/gzip"
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
)

type Options struct {
	ExcludedExtensions   ExcludedExtensions
	ExcludedPaths        ExcludedPaths
	ExcludedPathesRegexs ExcludedPathesRegexs
	DecompressFn         func(c *gin.Context)
	DecompressOnly       bool
}

type Option func(*Options)

// WithExcludedExtensions returns an Option that sets the ExcludedExtensions field of the Options struct.
// Parameters:
//   - args: []string - A slice of file extensions to exclude from gzip compression.
func WithExcludedExtensions(args []string) Option {
	return func(o *Options) {
		o.ExcludedExtensions = NewExcludedExtensions(args)
	}
}

// WithExcludedPaths returns an Option that sets the ExcludedPaths field of the Options struct.
// Parameters:
//   - args: []string - A slice of paths to exclude from gzip compression.
func WithExcludedPaths(args []string) Option {
	return func(o *Options) {
		o.ExcludedPaths = NewExcludedPaths(args)
	}
}

// WithExcludedPathsRegexs returns an Option that sets the ExcludedPathesRegexs field of the Options struct.
// Parameters:
//   - args: []string - A slice of regex patterns to exclude paths from gzip compression.
func WithExcludedPathsRegexs(args []string) Option {
	return func(o *Options) {
		o.ExcludedPathesRegexs = NewExcludedPathesRegexs(args)
	}
}

// WithDecompressFn returns an Option that sets the DecompressFn field of the Options struct.
// Parameters:
//   - decompressFn: func(c *gin.Context) - A function to handle decompression of incoming requests.
func WithDecompressFn(decompressFn func(c *gin.Context)) Option {
	return func(o *Options) {
		o.DecompressFn = decompressFn
	}
}

// WithDecompressOnly is an option that configures the gzip middleware to only
// decompress incoming requests without compressing the responses. When this
// option is enabled, the middleware will set the DecompressOnly field of the
// Options struct to true.
func WithDecompressOnly() Option {
	return func(o *Options) {
		o.DecompressOnly = true
	}
}

// Using map for better lookup performance
type ExcludedExtensions map[string]struct{}

// NewExcludedExtensions creates a new ExcludedExtensions map from a slice of file extensions.
// Parameters:
//   - extensions: []string - A slice of file extensions to exclude from gzip compression.
//
// Returns:
//   - ExcludedExtensions - A map of excluded file extensions.
func NewExcludedExtensions(extensions []string) ExcludedExtensions {
	res := make(ExcludedExtensions, len(extensions))
	for _, e := range extensions {
		res[e] = struct{}{}
	}
	return res
}

// Contains checks if a given file extension is in the ExcludedExtensions map.
// Parameters:
//   - target: string - The file extension to check.
//
// Returns:
//   - bool - True if the extension is excluded, false otherwise.
func (e ExcludedExtensions) Contains(target string) bool {
	_, ok := e[target]
	return ok
}

type ExcludedPaths []string

// NewExcludedPaths creates a new ExcludedPaths slice from a slice of paths.
// Parameters:
//   - paths: []string - A slice of paths to exclude from gzip compression.
//
// Returns:
//   - ExcludedPaths - A slice of excluded paths.
func NewExcludedPaths(paths []string) ExcludedPaths {
	return ExcludedPaths(paths)
}

// Contains checks if a given request URI starts with any of the excluded paths.
// Parameters:
//   - requestURI: string - The request URI to check.
//
// Returns:
//   - bool - True if the URI starts with an excluded path, false otherwise.
func (e ExcludedPaths) Contains(requestURI string) bool {
	for _, path := range e {
		if strings.HasPrefix(requestURI, path) {
			return true
		}
	}
	return false
}

type ExcludedPathesRegexs []*regexp.Regexp

// NewExcludedPathesRegexs creates a new ExcludedPathesRegexs slice from a slice of regex patterns.
// Parameters:
//   - regexs: []string - A slice of regex patterns to exclude paths from gzip compression.
//
// Returns:
//   - ExcludedPathesRegexs - A slice of excluded path regex patterns.
func NewExcludedPathesRegexs(regexs []string) ExcludedPathesRegexs {
	result := make(ExcludedPathesRegexs, len(regexs))
	for i, reg := range regexs {
		result[i] = regexp.MustCompile(reg)
	}
	return result
}

// Contains checks if a given request URI matches any of the excluded path regex patterns.
// Parameters:
//   - requestURI: string - The request URI to check.
//
// Returns:
//   - bool - True if the URI matches an excluded path regex pattern, false otherwise.
func (e ExcludedPathesRegexs) Contains(requestURI string) bool {
	for _, reg := range e {
		if reg.MatchString(requestURI) {
			return true
		}
	}
	return false
}

// DefaultDecompressHandle is a middleware function for the Gin framework that
// decompresses the request body if it is gzip encoded. It checks if the request
// body is nil and returns immediately if it is. Otherwise, it attempts to create
// a new gzip reader from the request body. If an error occurs during this process,
// it aborts the request with a 400 Bad Request status and the error. If successful,
// it removes the "Content-Encoding" and "Content-Length" headers from the request
// and replaces the request body with the decompressed reader.
//
// Parameters:
//   - c: *gin.Context - The Gin context for the current request.
func DefaultDecompressHandle(c *gin.Context) {
	if c.Request.Body == nil {
		return
	}
	r, err := gzip.NewReader(c.Request.Body)
	if err != nil {
		_ = c.AbortWithError(http.StatusBadRequest, err)
		return
	}
	c.Request.Header.Del("Content-Encoding")
	c.Request.Header.Del("Content-Length")
	c.Request.Body = r
}
