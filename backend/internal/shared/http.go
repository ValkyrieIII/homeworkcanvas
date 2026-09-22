package shared

import (
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
)

func RespondError(c *gin.Context, e error) {
	var ae *AppError
	if errors.As(e, &ae) {
		c.JSON(ae.Status, gin.H{"error": gin.H{"code": ae.Code, "message": ae.Message, "details": gin.H{}}})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "shared.internal", "message": "服务暂时不可用", "details": gin.H{}}})
}
