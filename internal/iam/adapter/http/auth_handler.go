package http

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"

	"oneweekonething.com/owot-pro/internal/iam/app"
	"oneweekonething.com/owot-pro/internal/iam/domain"
	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
	"oneweekonething.com/owot-pro/internal/platform/httpx"
)

func (h *Handler) Verify(ctx context.Context, token string) (ctxmeta.LoginUser, error) {
	user, err := h.service.Verify(ctx, token)
	if errors.Is(err, domain.ErrTokenInvalid) {
		return ctxmeta.LoginUser{}, httpx.ErrUnauthenticated
	}
	return user, err
}

func bearer(c *gin.Context) string {
	parts := strings.Fields(c.GetHeader("Authorization"))
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return parts[1]
	}
	return c.Query("token")
}
func loginResponse(v app.LoginResponse) gin.H {
	return gin.H{"userId": v.UserID, "accessToken": v.AccessToken, "refreshToken": v.RefreshToken, "expiresTime": v.ExpiresTime.UnixMilli()}
}

type menuNodeDTO struct {
	menuDTO
	Children []menuNodeDTO `json:"children"`
}

func menuTree(menus []domain.Menu) []menuNodeDTO {
	menus = slices.Clone(menus)
	slices.SortStableFunc(menus, func(a, b domain.Menu) int {
		if a.Sort < b.Sort {
			return -1
		}
		if a.Sort > b.Sort {
			return 1
		}
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	var convert func([]domain.MenuNode) []menuNodeDTO
	convert = func(nodes []domain.MenuNode) []menuNodeDTO {
		result := []menuNodeDTO{}
		for _, n := range nodes {
			result = append(result, menuNodeDTO{menuDTO: menuResponse(n.Menu), Children: convert(n.Children)})
		}
		return result
	}
	return convert(domain.MenuTree(menus))
}
func (h *Handler) registerAuth(r *httpx.Router) {
	s := h.service
	publicAuth := httpx.Route{PermitAll: true, CredentialExchange: true}
	r.Handle(http.MethodPost, prefix+"auth/login", publicAuth, func(c *gin.Context) {
		var dto struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if c.ShouldBindJSON(&dto) != nil {
			h.fail(c, domain.ErrInvalidArgument)
			return
		}
		v, err := s.Login(c.Request.Context(), app.LoginRequest{Username: dto.Username, Password: dto.Password, UserIP: c.ClientIP(), UserAgent: c.Request.UserAgent()})
		h.complete(c, loginResponse(v), err)
	})
	r.Handle(http.MethodPost, prefix+"auth/refresh-token", publicAuth, func(c *gin.Context) {
		v, err := s.Refresh(c.Request.Context(), c.Query("refreshToken"))
		h.complete(c, loginResponse(v), err)
	})
	r.Handle(http.MethodPost, prefix+"auth/logout", publicAuth, func(c *gin.Context) { h.done(c, s.Logout(c.Request.Context(), bearer(c))) })
	h.route(r, http.MethodGet, "auth/get-permission-info", "", func(c *gin.Context) {
		u, _ := ctxmeta.User(c.Request.Context())
		info, err := s.Snapshot(ctxmeta.WithTenantID(c.Request.Context(), u.TenantID), u.ID)
		if err != nil {
			h.fail(c, err)
			return
		}
		roles, permissions := []string{}, []string{}
		for _, r := range info.Roles {
			roles = append(roles, r.Code)
		}
		for _, m := range info.Menus {
			if m.Permission != "" && !slices.Contains(permissions, m.Permission) {
				permissions = append(permissions, m.Permission)
			}
		}
		if u.SuperAdmin {
			permissions = append(permissions, "*:*:*")
		}
		h.complete(c, gin.H{"user": gin.H{"id": info.User.ID, "nickname": info.User.Nickname, "avatar": info.User.Avatar}, "roles": roles, "permissions": permissions, "menus": menuTree(info.Menus)}, nil)
	})
	h.route(r, http.MethodGet, "auth/list-menus", "", func(c *gin.Context) {
		u, _ := ctxmeta.User(c.Request.Context())
		info, err := s.Snapshot(ctxmeta.WithTenantID(c.Request.Context(), u.TenantID), u.ID)
		h.complete(c, menuTree(info.Menus), err)
	})
}
