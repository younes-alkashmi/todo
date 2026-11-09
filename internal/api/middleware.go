package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	fb "github.com/younes-alkashmi/todo/internal/firebase"
)

type ctxKey string

const uidKey ctxKey = "uid"

func AuthMiddleware(fbClient *fb.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		idToken := strings.TrimPrefix(h, "Bearer ")
		token, err := fbClient.Auth.VerifyIDToken(c.Request.Context(), idToken)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		c.Set(string(uidKey), token.UID)
		c.Next()
	}
}

func UID(c *gin.Context) string {
	v, _ := c.Get(string(uidKey))
	s, _ := v.(string)
	return s
}
