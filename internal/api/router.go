package api

import (
	"github.com/gin-gonic/gin"
	fb "github.com/younes-alkashmi/todo/internal/firebase"
)

func NewRouter(fbClient *fb.Client) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	r.GET("/healthz", Health)

	auth := r.Group("/")
	auth.Use(AuthMiddleware(fbClient))
	{
		auth.GET("/me", Me)
		auth.POST("/sync/push", func(c *gin.Context) { Push(c, fbClient.Firestore) })
		auth.GET("/sync/pull", func(c *gin.Context) { Pull(c, fbClient.Firestore) })
	}

	return r
}
