package shared

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequestLogCorrelatesWithoutLoggingQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var output bytes.Buffer
	before := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(before) })
	r := gin.New()
	r.Use(RequestLog())
	r.GET("/items/:id", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/items/secret-id?token=secret-token", nil))
	var event map[string]any
	if err := json.Unmarshal(output.Bytes(), &event); err != nil {
		t.Fatal(err)
	}
	id := w.Header().Get("X-Request-ID")
	if id == "" || event["request_id"] != id || event["route"] != "/items/:id" || event["status"] != float64(204) {
		t.Fatalf("unexpected request event: %v", event)
	}
	if bytes.Contains(output.Bytes(), []byte("secret")) {
		t.Fatalf("request log leaked URL data: %s", output.Bytes())
	}
}

func TestSafeRecoveryDoesNotLogRequestHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var output bytes.Buffer
	before := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(before) })
	r := gin.New()
	r.Use(RequestLog(), SafeRecovery())
	r.GET("/panic", func(c *gin.Context) { panic("private panic text") })
	w := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/panic?token=private-query", nil)
	request.Header.Set("X-Canvas-Subject", "private-subject")
	r.ServeHTTP(w, request)
	if w.Code != http.StatusInternalServerError || w.Header().Get("X-Request-ID") == "" {
		t.Fatalf("status=%d request ID=%q", w.Code, w.Header().Get("X-Request-ID"))
	}
	if bytes.Contains(output.Bytes(), []byte("private")) {
		t.Fatalf("recovery log leaked request data: %s", output.Bytes())
	}
}
