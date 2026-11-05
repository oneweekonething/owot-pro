package http

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"oneweekonething.com/owot-pro/internal/iam/app"

	"oneweekonething.com/owot-pro/internal/iam/domain"
	"oneweekonething.com/owot-pro/internal/platform/httpx"
)

func (h *Handler) Register(r *httpx.Router) {
	s := h.service
	registerResource(h, r, resource[domain.Dept, deptDTO]{Path: "dept", Get: s.GetDept, List: s.ListDepts, Save: s.SaveDept, Delete: s.DeleteDepts, Response: deptResponse, ID: func(v deptDTO) int64 { return v.ID }, ListRoute: true, Batch: true, Simple: true})
	registerResource(h, r, resource[domain.Post, postDTO]{Path: "post", Get: s.GetPost, List: s.PagePosts, Save: s.SavePost, Delete: s.DeletePosts, Response: postResponse, ID: func(v postDTO) int64 { return v.ID }, PageRoute: true, Batch: true, Simple: true, Export: true, ExportRow: func(v domain.Post) []any { return []any{v.ID, v.Name, v.Code, v.Status} }})
	registerResource(h, r, resource[domain.Role, roleDTO]{Path: "role", Get: s.GetRole, List: s.PageRoles, Save: s.SaveRole, Delete: s.DeleteRoles, Response: roleResponse, ID: func(v roleDTO) int64 { return v.ID }, PageRoute: true, Batch: true, Simple: true, Export: true, ExportRow: func(v domain.Role) []any { return []any{v.ID, v.Name, v.Code, v.Status} }})
	registerResource(h, r, resource[domain.Menu, menuDTO]{Path: "menu", Get: s.GetMenu, List: s.ListMenus, Save: s.SaveMenu, Delete: func(ctx context.Context, ids []int64) error { return s.DeleteMenu(ctx, ids[0]) }, Response: menuResponse, ID: func(v menuDTO) int64 { return v.ID }, ListRoute: true, Simple: true})
	registerResource(h, r, resource[domain.TenantPackage, packageDTO]{Path: "tenant-package", Get: s.GetPackage, List: s.PagePackages, Save: s.SavePackage, Delete: s.DeletePackages, Response: packageResponse, ID: func(v packageDTO) int64 { return v.ID }, PageRoute: true, Batch: true, Simple: true})
	registerResource(h, r, resource[domain.OAuth2Client, clientDTO]{Path: "oauth2-client", Get: s.GetClient, List: s.PageClients, Save: s.SaveClient, Delete: func(ctx context.Context, ids []int64) error { return s.DeleteClient(ctx, ids[0]) }, Response: clientResponse, ID: func(v clientDTO) int64 { return v.ID }, PageRoute: true})
	h.route(r, http.MethodPut, "role/update-status", "system:role:update", func(c *gin.Context) {
		var dto struct {
			ID     int64 `json:"id"`
			Status *int  `json:"status"`
		}
		if c.ShouldBindJSON(&dto) != nil || dto.ID <= 0 || dto.Status == nil {
			h.fail(c, domain.ErrInvalidArgument)
			return
		}
		h.done(c, s.UpdateRoleStatus(c.Request.Context(), dto.ID, *dto.Status))
	})
	h.registerAuth(r)
	h.registerUsers(r)
	h.registerPermission(r)
	h.registerTenants(r)
	h.route(r, http.MethodGet, "oauth2-token/page", "system:oauth2-token:page", func(c *gin.Context) {
		q, err := query(c)
		if err != nil {
			h.fail(c, err)
			return
		}
		page, err := s.PageTokens(c.Request.Context(), q)
		h.complete(c, mapPage(page, func(t domain.AccessToken) gin.H {
			return gin.H{"id": t.ID, "userId": t.UserID, "userType": t.UserType, "clientId": t.ClientID, "accessToken": "session:" + strconv.FormatInt(t.ID, 10), "createTime": t.CreateTime.UnixMilli(), "expiresTime": t.ExpiresTime.UnixMilli()}
		}), err)
	})
	h.route(r, http.MethodDelete, "oauth2-token/delete", "system:oauth2-token:delete", func(c *gin.Context) {
		token := c.Query("accessToken")
		if token == "" {
			token = c.Query("token")
		}
		selector := app.AccessSelector{Token: token}
		if raw, ok := strings.CutPrefix(token, "session:"); ok {
			id, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || id <= 0 {
				h.fail(c, domain.ErrInvalidArgument)
				return
			}
			selector = app.AccessSelector{ID: id}
		}
		h.done(c, s.ForceLogout(c.Request.Context(), selector))
	})
}
