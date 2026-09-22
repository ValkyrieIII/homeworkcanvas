package identity

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"homeworkcanvas/backend/internal/shared"
	"net/http"
)

const UserKey = "current_user"

// HeaderIdentity is a local development adapter. Production replaces it with UniSSO verification.
func HeaderIdentity(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		subject := c.GetHeader("X-Canvas-Subject")
		name := c.GetHeader("X-Canvas-Name")
		if subject == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "identity.unauthenticated", "message": "缺少登录身份"}})
			return
		}
		if name == "" {
			name = subject
		}
		var u shared.User
		err := db.Where("external_subject = ?", subject).First(&u).Error
		if err == gorm.ErrRecordNotFound {
			u = shared.User{ExternalSubject: subject, DisplayName: name, Status: "active"}
			err = db.Create(&u).Error
		}
		if err != nil {
			c.AbortWithStatusJSON(500, gin.H{"error": gin.H{"code": "shared.database_error", "message": "无法读取用户信息"}})
			return
		}
		if u.Status != "active" {
			c.AbortWithStatusJSON(403, gin.H{"error": gin.H{"code": "identity.disabled", "message": "账号已禁用"}})
			return
		}
		c.Set(UserKey, u)
		c.Next()
	}
}
func Current(c *gin.Context) shared.User { return c.MustGet(UserKey).(shared.User) }
