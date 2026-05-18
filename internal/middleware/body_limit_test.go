package middleware

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBodyLimit_UnderLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(BodyLimit(1024)) // 1KB limit
	r.POST("/api/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	body := `{"name": "test", "value": "small payload"}`
	req := httptest.NewRequest("POST", "/api/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200 for small body, got %d", w.Code)
	}
}

func TestBodyLimit_OverLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(BodyLimit(10)) // 10 bytes limit
	r.POST("/api/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	body := `{"this is a long payload exceeding the limit"}`
	req := httptest.NewRequest("POST", "/api/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 413 {
		t.Errorf("expected 413 for oversized body, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestBodyLimit_SkipsGetRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(BodyLimit(5)) // 5 bytes — very restrictive
	r.GET("/api/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200 for GET (body limit skipped), got %d", w.Code)
	}
}

func TestBodyLimit_SkipsHeadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(BodyLimit(5))
	r.HEAD("/api/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	req := httptest.NewRequest("HEAD", "/api/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200 for HEAD (body limit skipped), got %d", w.Code)
	}
}
