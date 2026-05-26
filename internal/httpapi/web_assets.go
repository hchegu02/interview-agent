package httpapi

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed web/index.html web/assets/*
var webAssets embed.FS

func (s *Server) registerWebRoutes(r *gin.Engine) {
	r.GET("/", func(c *gin.Context) {
		index, err := webAssets.ReadFile("web/index.html")
		if err != nil {
			panic(err)
		}
		c.Header("Cache-Control", "no-store")
		c.Data(http.StatusOK, "text/html; charset=utf-8", index)
	})

	assets, err := fs.Sub(webAssets, "web/assets")
	if err != nil {
		panic(err)
	}
	r.GET("/assets/*filepath", gin.WrapH(http.StripPrefix("/assets/", http.FileServer(http.FS(assets)))))
}
