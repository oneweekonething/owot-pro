package httpx

import (
	"net/http"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
)

// cors permits bearer-token clients without enabling cookie credentials.
// An empty origin list permits any origin; configured origins are matched exactly.
func cors(origins []string) gin.HandlerFunc {
	allowed := make([]string, 0, len(origins))
	for _, origin := range origins {
		if origin = strings.TrimSpace(origin); origin != "" {
			allowed = append(allowed, origin)
		}
	}
	methods := []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	headers := []string{"authorization", "content-type", "tenant-id", "visit-tenant-id", "trace-id"}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" {
			c.Next()
			return
		}
		c.Writer.Header().Add("Vary", "Origin")
		if len(allowed) == 0 || slices.Contains(allowed, "*") {
			c.Header("Access-Control-Allow-Origin", "*")
		} else if slices.Contains(allowed, origin) {
			c.Header("Access-Control-Allow-Origin", origin)
		} else {
			Failure(c, 403, "不允许该来源跨域访问")
			return
		}
		c.Header("Access-Control-Expose-Headers", "trace-id, Content-Disposition")
		if c.Request.Method != http.MethodOptions || c.GetHeader("Access-Control-Request-Method") == "" {
			c.Next()
			return
		}
		c.Writer.Header().Add("Vary", "Access-Control-Request-Method")
		c.Writer.Header().Add("Vary", "Access-Control-Request-Headers")
		if !slices.Contains(methods, c.GetHeader("Access-Control-Request-Method")) {
			Failure(c, 405, "不支持该跨域请求方法")
			return
		}
		for _, header := range strings.Split(c.GetHeader("Access-Control-Request-Headers"), ",") {
			header = strings.ToLower(strings.TrimSpace(header))
			if header != "" && !slices.Contains(headers, header) {
				Failure(c, 403, "不支持该跨域请求头")
				return
			}
		}
		c.Header("Access-Control-Allow-Methods", strings.Join(methods, ", "))
		c.Header("Access-Control-Allow-Headers", strings.Join(headers, ", "))
		Success(c, true)
		c.Abort()
	}
}
