package shared

import "net/http"

type AppError struct {
	Code, Message string
	Status        int
}

func (e *AppError) Error() string                          { return e.Code + ": " + e.Message }
func JSONError(code, message string, status int) *AppError { return &AppError{code, message, status} }

var ErrUnauthenticated = JSONError("identity.unauthenticated", "未登录或登录已失效", http.StatusUnauthorized)
