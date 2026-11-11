package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func Test图片路径不影响应用路由(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(ReadableFileHandler(func(c *gin.Context) { c.Status(418); c.Abort() }))
	router.NoRoute(func(c *gin.Context) { c.Status(404) })
	router.GET("/home", func(c *gin.Context) { c.Status(200) })
	router.GET("/api/v4/site/ping", func(c *gin.Context) { c.Status(200) })
	for _, tc := range []struct {
		method, path string
		code         int
	}{
		{"GET", "/common/照片.png", 418}, {"HEAD", "/events/2026/a.png", 418}, {"OPTIONS", "/events/a.png", 204},
		{"GET", "/home", 200}, {"GET", "/api/v4/site/ping", 200}, {"GET", "/f/6juk/a.png", 404},
		{"GET", "/events/../a.png", 404}, {"GET", "/events//a.png", 404},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
			require.Equal(t, tc.code, w.Code)
			if tc.code == 418 {
				require.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
			}
		})
	}
}
