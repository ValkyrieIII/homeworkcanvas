package shared

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"net/http"
)

func RespondError(c *gin.Context, e error) {
	ae, explicit := classify(e)
	if !explicit {
		LogHTTPError(c, ae.Code, ae.Status, e)
	}
	c.JSON(ae.Status, gin.H{"error": gin.H{"code": ae.Code, "message": ae.Message, "details": gin.H{}}})
}

// classify 把数据库错误翻译成对用户有意义的响应。约束冲突、外键缺失、字段格式错误
// 都是调用方的问题，塌成 500 会让用户和排查者都以为是服务故障。
// 第二个返回值表示原错误是否已经自带明确分类（是则不必记日志）。
func classify(e error) (*AppError, bool) {
	var ae *AppError
	if errors.As(e, &ae) {
		return ae, true
	}
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return JSONError("shared.not_found", "记录不存在", http.StatusNotFound), false
	}
	var pg *pgconn.PgError
	if errors.As(e, &pg) {
		switch pg.Code {
		case "23505":
			return JSONError("shared.conflict", "记录已存在", http.StatusConflict), false
		case "23503":
			return JSONError("shared.invalid_reference", "引用的记录不存在", http.StatusBadRequest), false
		case "23502":
			return JSONError("shared.missing_field", "缺少必填字段", http.StatusBadRequest), false
		case "22P02", "22007", "22008":
			return JSONError("shared.invalid_value", "字段格式不合法", http.StatusBadRequest), false
		}
	}
	return JSONError("shared.internal", "服务暂时不可用", http.StatusInternalServerError), false
}
