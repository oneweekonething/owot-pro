package http

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"oneweekonething.com/owot-pro/internal/iam/domain"
	"oneweekonething.com/owot-pro/internal/platform/httpx"
)

func (h *Handler) registerPermission(r *httpx.Router) {
	s := h.service
	h.route(r, http.MethodGet, "permission/list-role-menus", "system:permission:assign-role-menu", func(c *gin.Context) {
		id, err := idParam(c, "roleId")
		if err != nil {
			h.fail(c, err)
			return
		}
		ids, err := s.RoleMenuIDs(c.Request.Context(), id)
		h.complete(c, nonNil(ids), err)
	})
	h.route(r, http.MethodGet, "permission/list-user-roles", "system:permission:assign-user-role", func(c *gin.Context) {
		id, err := idParam(c, "userId")
		if err != nil {
			h.fail(c, err)
			return
		}
		ids, err := s.UserRoleIDs(c.Request.Context(), id)
		h.complete(c, nonNil(ids), err)
	})
	h.route(r, http.MethodPost, "permission/assign-role-menu", "system:permission:assign-role-menu", func(c *gin.Context) {
		var dto struct {
			RoleID  int64   `json:"roleId"`
			MenuIDs []int64 `json:"menuIds"`
		}
		if c.ShouldBindJSON(&dto) != nil || dto.RoleID <= 0 {
			h.fail(c, domain.ErrInvalidArgument)
			return
		}
		h.done(c, s.AssignRoleMenus(c.Request.Context(), dto.RoleID, dto.MenuIDs))
	})
	h.route(r, http.MethodPost, "permission/assign-user-role", "system:permission:assign-user-role", func(c *gin.Context) {
		var dto struct {
			UserID  int64   `json:"userId"`
			RoleIDs []int64 `json:"roleIds"`
		}
		if c.ShouldBindJSON(&dto) != nil || dto.UserID <= 0 {
			h.fail(c, domain.ErrInvalidArgument)
			return
		}
		h.done(c, s.AssignUserRoles(c.Request.Context(), dto.UserID, dto.RoleIDs))
	})
	h.route(r, http.MethodPost, "permission/assign-role-data-scope", "system:permission:assign-role-data-scope", func(c *gin.Context) {
		var dto struct {
			RoleID           int64   `json:"roleId"`
			DataScope        int     `json:"dataScope"`
			DataScopeDeptIDs []int64 `json:"dataScopeDeptIds"`
		}
		if c.ShouldBindJSON(&dto) != nil || dto.RoleID <= 0 {
			h.fail(c, domain.ErrInvalidArgument)
			return
		}
		h.done(c, s.AssignDataScope(c.Request.Context(), dto.RoleID, dto.DataScope, dto.DataScopeDeptIDs))
	})
}
