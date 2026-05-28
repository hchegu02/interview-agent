package httpapi

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed web/dist/index.html web/dist/assets/*
var webAssets embed.FS

func (s *Server) registerWebRoutes(r *gin.Engine) {
	serveIndex := func(c *gin.Context) {
		index, err := webAssets.ReadFile("web/dist/index.html")
		if err != nil {
			panic(err)
		}
		c.Header("Cache-Control", "no-store")
		c.Data(http.StatusOK, "text/html; charset=utf-8", index)
	}

	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusTemporaryRedirect, "/resume")
	})
	for _, path := range []string{"/resume", "/jd", "/interview", "/report", "/questions"} {
		r.GET(path, serveIndex)
	}

	assets, err := fs.Sub(webAssets, "web/dist/assets")
	if err != nil {
		panic(err)
	}
	fileServer := http.StripPrefix("/assets/", http.FileServer(http.FS(assets)))
	r.GET("/assets/*filepath", func(c *gin.Context) {
		if strings.HasSuffix(c.Request.URL.Path, ".js") || strings.HasSuffix(c.Request.URL.Path, ".css") {
			c.Header("Cache-Control", "public, max-age=31536000, immutable")
		}
		fileServer.ServeHTTP(c.Writer, c.Request)
	})
}
