package middleware

import (
	"net/http"
	"strings"

	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/gin-gonic/gin"
)

// ReadableFileHandler 在前端页面回退之前处理图片路径，避免不存在的图片返回 HTML。
func ReadableFileHandler(image gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		parts := strings.Split(strings.TrimPrefix(c.Request.URL.Path, "/"), "/")
		if len(parts) < 2 || inventory.IsApplicationURLName(parts[0]) {
			c.Next()
			return
		}
		if inventory.ValidateFolderURLName(parts[0], false) != nil {
			c.Next()
			return
		}
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead && c.Request.Method != http.MethodOptions {
			c.Next()
			return
		}
		c.Header("Access-Control-Allow-Origin", "*")
		if c.Request.Method == http.MethodOptions {
			c.Header("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
			c.Status(http.StatusNoContent)
			c.Abort()
			return
		}
		for _, part := range parts {
			if part == "" || part == "." || part == ".." {
				c.Status(http.StatusNotFound)
				c.Abort()
				return
			}
		}
		image(c)
	}
}
