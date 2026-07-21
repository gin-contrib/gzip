# GZIP gin's middleware

[![Run Tests](https://github.com/gin-contrib/gzip/actions/workflows/go.yml/badge.svg)](https://github.com/gin-contrib/gzip/actions/workflows/go.yml)
[![Trivy Security Scan](https://github.com/gin-contrib/gzip/actions/workflows/trivy-scan.yml/badge.svg)](https://github.com/gin-contrib/gzip/actions/workflows/trivy-scan.yml)
[![codecov](https://codecov.io/gh/gin-contrib/gzip/branch/master/graph/badge.svg)](https://codecov.io/gh/gin-contrib/gzip)
[![Go Report Card](https://goreportcard.com/badge/github.com/gin-contrib/gzip)](https://goreportcard.com/report/github.com/gin-contrib/gzip)
[![GoDoc](https://godoc.org/github.com/gin-contrib/gzip?status.svg)](https://godoc.org/github.com/gin-contrib/gzip)

Gin middleware to enable `GZIP` support.

## Usage

Download and install it:

```sh
go get github.com/gin-contrib/gzip
```

Import it in your code:

```go
import "github.com/gin-contrib/gzip"
```

Canonical example:

```go
package main

import (
  "fmt"
  "net/http"
  "time"

  "github.com/gin-contrib/gzip"
  "github.com/gin-gonic/gin"
)

func main() {
  r := gin.Default()
  r.Use(gzip.Gzip(gzip.DefaultCompression))
  r.GET("/ping", func(c *gin.Context) {
    c.String(http.StatusOK, "pong "+fmt.Sprint(time.Now().Unix()))
  })

  // Listen and Server in 0.0.0.0:8080
  if err := r.Run(":8080"); err != nil {
    log.Fatal(err)
  }
}
```

### Compress only when response meets minimum byte size

```go
package main

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()
	r.Use(gzip.Gzip(gzip.DefaultCompression, gzip.WithMinLength(2048)))
	r.GET("/ping", func(c *gin.Context) {
		sizeStr := c.Query("size")
		size, _ := strconv.Atoi(sizeStr)
		c.String(http.StatusOK, strings.Repeat("a", size))
	})

	// Listen and Server in 0.0.0.0:8080
	if err := r.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}
```
Test with curl:
```bash
curl -i --compressed 'http://localhost:8080/ping?size=2047'
curl -i --compressed 'http://localhost:8080/ping?size=2048'
```

Notes:
- If a "Content-Length" header is set, that will be used to determine whether to compress based on the given min length.
- If no "Content-Length" header is set, a buffer is used to temporarily store writes until the min length is met or the request completes.
  - Setting a high min length will result in more buffering (2048 bytes is a recommended default for most cases)
  - The handler performs optimizations to avoid unnecessary operations, such as testing if `len(data)` exceeds min length before writing to the buffer, and reusing buffers between requests.

### Customized Excluded Extensions

```go
package main

import (
  "fmt"
  "net/http"
  "time"

  "github.com/gin-contrib/gzip"
  "github.com/gin-gonic/gin"
)

func main() {
  r := gin.Default()
  r.Use(gzip.Gzip(gzip.DefaultCompression, gzip.WithExcludedExtensions([]string{".pdf", ".mp4"})))
  r.GET("/ping", func(c *gin.Context) {
    c.String(http.StatusOK, "pong "+fmt.Sprint(time.Now().Unix()))
  })

  // Listen and Server in 0.0.0.0:8080
  if err := r.Run(":8080"); err != nil {
    log.Fatal(err)
  }
}
```

### Customized Excluded Paths

```go
package main

import (
  "fmt"
  "net/http"
  "time"

  "github.com/gin-contrib/gzip"
  "github.com/gin-gonic/gin"
)

func main() {
  r := gin.Default()
  r.Use(gzip.Gzip(gzip.DefaultCompression, gzip.WithExcludedPaths([]string{"/api/"})))
  r.GET("/ping", func(c *gin.Context) {
    c.String(http.StatusOK, "pong "+fmt.Sprint(time.Now().Unix()))
  })

  // Listen and Server in 0.0.0.0:8080
  if err := r.Run(":8080"); err != nil {
    log.Fatal(err)
  }
}
```

### Customized Excluded Paths with Regex

```go
package main

import (
  "fmt"
  "net/http"
  "time"

  "github.com/gin-contrib/gzip"
  "github.com/gin-gonic/gin"
)

func main() {
  r := gin.Default()
  r.Use(gzip.Gzip(gzip.DefaultCompression, gzip.WithExcludedPathsRegexs([]string{".*"})))
  r.GET("/ping", func(c *gin.Context) {
    c.String(http.StatusOK, "pong "+fmt.Sprint(time.Now().Unix()))
  })

  // Listen and Server in 0.0.0.0:8080
  if err := r.Run(":8080"); err != nil {
    log.Fatal(err)
  }
}
```

### Customized Should Compress Function

```go
package main

import (
	"net/http"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()

	// Custom logic:
	//   Only compress responses when the request has header X-Allow-Compress: true
	customFn := func(c *gin.Context) bool {
		return c.GetHeader("X-Allow-Compress") == "true"
	}

	r.Use(gzip.Gzip(
		gzip.DefaultCompression,
		gzip.WithExcludedPaths([]string{"/api/"}),
		gzip.WithExcludedPathsRegexs([]string{".*"}),
		gzip.WithCustomShouldCompressFn(customFn),
		gzip.WithCombineDefaultAndCustom(), // <-- combine default + custom rules
	))

	r.GET("/data", func(c *gin.Context) {
		c.String(http.StatusOK, "compressed response data")
	})

	if err := r.Run(":8080"); err != nil {
		panic(err)
	}
}
```
### Server Push

```go
package main

import (
  "fmt"
  "log"
  "net/http"
  "time"

  "github.com/gin-contrib/gzip"
  "github.com/gin-gonic/gin"
)

func main() {
  r := gin.Default()
  r.Use(gzip.Gzip(gzip.DefaultCompression))
  r.GET("/stream", func(c *gin.Context) {
    c.Header("Content-Type", "text/event-stream")
    c.Header("Connection", "keep-alive")
    for i := 0; i < 10; i++ {
      fmt.Fprintf(c.Writer, "id: %d\ndata: tick %d\n\n", i, time.Now().Unix())
      c.Writer.Flush()
      time.Sleep(1 * time.Second)
    }
  })

  // Listen and Server in 0.0.0.0:8080
  if err := r.Run(":8080"); err != nil {
    log.Fatal(err)
  }
}
```
