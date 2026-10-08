package httpx

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func wrapHTTP(h http.HandlerFunc) gin.HandlerFunc { return gin.WrapF(h) }
