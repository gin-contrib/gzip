package main

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-contrib/gzip"

	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()
	r.Use(
		func(c *gin.Context) {
			c.Writer.Header().Add("Vary", "Origin")
		},
		gzip.Gzip(
			gzip.DefaultCompression,
			gzip.WithExcludedPaths([]string{"/ping2"}),
		),
	)

	r.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong "+strconv.FormatInt(time.Now().Unix(), 10))
	})
	r.GET("/ping2", func(c *gin.Context) {
		c.String(http.StatusOK, "pong "+strconv.FormatInt(time.Now().Unix(), 10))
	})
	r.GET("/stream", func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Connection", "keep-alive")
		for i := range 10 {
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
