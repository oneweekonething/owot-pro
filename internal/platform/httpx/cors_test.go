package httpx

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCORSOriginAllowlistAndPreflightValidation(t *testing.T) {
	router := NewRouter(Dependencies{AllowedOrigins: []string{" https://admin.example.com "}})
	router.Handle("GET", "/public", Route{PermitAll: true, TenantIgnore: true}, func(c *gin.Context) { Success(c, true) })
	for _, tt := range []struct {
		name, origin, method, requestedMethod, requestedHeaders string
		code                                                    int
		allowOrigin                                             string
	}{
		{"allowed", "https://admin.example.com", "GET", "", "", 0, "https://admin.example.com"},
		{"no origin", "", "GET", "", "", 0, ""},
		{"rejected origin", "https://other.example.com", "GET", "", "", 403, ""},
		{"allowed preflight", "https://admin.example.com", "OPTIONS", "PUT", "Authorization, Tenant-Id, Content-Type", 0, "https://admin.example.com"},
		{"rejected method", "https://admin.example.com", "OPTIONS", "TRACE", "", 405, "https://admin.example.com"},
		{"rejected header", "https://admin.example.com", "OPTIONS", "GET", "X-Unknown", 403, "https://admin.example.com"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/public", nil)
			req.Header.Set("Origin", tt.origin)
			req.Header.Set("Access-Control-Request-Method", tt.requestedMethod)
			req.Header.Set("Access-Control-Request-Headers", tt.requestedHeaders)
			w := httptest.NewRecorder()
			router.Engine.ServeHTTP(w, req)
			var result struct{ Code int }
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != 200 || result.Code != tt.code {
				t.Fatalf("response = %d %s, err=%v", w.Code, w.Body.String(), err)
			}
			if w.Header().Get("Access-Control-Allow-Origin") != tt.allowOrigin {
				t.Fatalf("unexpected allowed origin: %v", w.Header())
			}
			if w.Header().Get("Access-Control-Allow-Credentials") != "" {
				t.Fatal("cookie credentials unexpectedly enabled")
			}
			if tt.allowOrigin != "" && !strings.Contains(w.Header().Get("Access-Control-Expose-Headers"), "Content-Disposition") {
				t.Fatal("download filename header not exposed")
			}
		})
	}
}
