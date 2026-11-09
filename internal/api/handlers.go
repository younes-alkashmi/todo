package api

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

func Health(c *gin.Context) {
	c.String(http.StatusOK, "ok")
}

func Me(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"uid": UID(c)})
}
