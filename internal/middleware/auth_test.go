package middleware

import (
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"mcp-gateway-go-demo/internal/model"
	"mcp-gateway-go-demo/internal/repository"
)

func setupAuthTestDB(t *testing.T) *repository.ApiToolRepo {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	db.AutoMigrate(&model.Gateway{}, &model.ApiKey{})
	repo := repository.NewApiToolRepo(db)
	return repo
}

func TestAPIKeyAuth_Disabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := setupAuthTestDB(t)

	var rtCfgPtr atomic.Value
	rtCfgPtr.Store(&RuntimeConfig{AuthEnabled: false})

	r := gin.New()
	r.Use(APIKeyAuth(AuthConfig{Enabled: false, ExemptPaths: []string{}}, repo, &rtCfgPtr))
	r.GET("/api/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200 when auth disabled, got %d", w.Code)
	}
}

func TestAPIKeyAuth_ExemptPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := setupAuthTestDB(t)

	r := gin.New()
	r.Use(APIKeyAuth(AuthConfig{
		Enabled: true, ExemptPaths: []string{"/metrics", "/api/health"},
	}, repo, nil))
	r.GET("/metrics", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })
	r.GET("/api/health", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	for _, path := range []string{"/metrics", "/api/health"} {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Errorf("expected 200 for exempt path %s, got %d", path, w.Code)
		}
	}
}

func TestAPIKeyAuth_MissingKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := setupAuthTestDB(t)
	// 创建默认网关（不要求 API Key）
	repo.CreateGateway(&model.Gateway{Name: "Default Gateway"})

	r := gin.New()
	r.Use(APIKeyAuth(AuthConfig{Enabled: true, ExemptPaths: []string{"/"}}, repo, nil))
	r.GET("/api/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200 for public gateway without key, got %d", w.Code)
	}
}

func TestAPIKeyAuth_InvalidKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := setupAuthTestDB(t)

	r := gin.New()
	r.Use(APIKeyAuth(AuthConfig{Enabled: true, ExemptPaths: []string{}}, repo, nil))
	r.GET("/api/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-API-Key", "invalid-key")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Errorf("expected 401 for invalid key, got %d", w.Code)
	}
}

func TestAPIKeyAuth_ValidKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := setupAuthTestDB(t)

	gw := &model.Gateway{Name: "Test GW", APIKeyRequired: true}
	repo.CreateGateway(gw)
	repo.CreateApiKey(&model.ApiKey{GatewayID: gw.ID, Key: "valid-key", Name: "test"})

	r := gin.New()
	r.Use(APIKeyAuth(AuthConfig{Enabled: true, ExemptPaths: []string{}}, repo, nil))
	r.GET("/api/test", func(c *gin.Context) {
		gwID, _ := c.Get("gateway_id")
		c.JSON(200, gin.H{"gateway_id": gwID})
	})

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-API-Key", "valid-key")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200 for valid key, got %d", w.Code)
	}
}

func TestAPIKeyAuth_QueryParamKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := setupAuthTestDB(t)

	gw := &model.Gateway{Name: "Query GW", APIKeyRequired: true}
	repo.CreateGateway(gw)
	repo.CreateApiKey(&model.ApiKey{GatewayID: gw.ID, Key: "query-key", Name: "test"})

	r := gin.New()
	r.Use(APIKeyAuth(AuthConfig{Enabled: true, ExemptPaths: []string{}}, repo, nil))
	r.GET("/api/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	req := httptest.NewRequest("GET", "/api/test?api_key=query-key", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200 for query param key, got %d", w.Code)
	}
}

func TestAPIKeyAuth_GatewayRequiresKey_NoKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := setupAuthTestDB(t)

	repo.CreateGateway(&model.Gateway{Name: "Secure GW", APIKeyRequired: true})

	r := gin.New()
	r.Use(APIKeyAuth(AuthConfig{Enabled: true, ExemptPaths: []string{}}, repo, nil))
	r.GET("/api/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	req := httptest.NewRequest("GET", "/api/test?gateway=Secure+GW", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Errorf("expected 401 for gateway requiring key, got %d", w.Code)
	}
}

func TestAPIKeyAuth_HotReloadDisable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := setupAuthTestDB(t)

	var rtCfgPtr atomic.Value
	rtCfgPtr.Store(&RuntimeConfig{AuthEnabled: true})

	r := gin.New()
	r.Use(APIKeyAuth(AuthConfig{Enabled: true, ExemptPaths: []string{"/"}}, repo, &rtCfgPtr))
	r.GET("/api/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	// 热更新：关闭认证
	rtCfgPtr.Store(&RuntimeConfig{AuthEnabled: false})

	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200 after hot-reload disabled auth, got %d", w.Code)
	}
}
