package http

import (
	"context"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"oneweekonething.com/owot-pro/internal/iam/app"
	"oneweekonething.com/owot-pro/internal/iam/domain"
	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
	"oneweekonething.com/owot-pro/internal/platform/httpx"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
)

type userDetailDTO struct {
	userDTO
	DeptName string    `json:"deptName"`
	Dept     *deptDTO  `json:"dept"`
	Roles    []roleDTO `json:"roles"`
	Posts    []postDTO `json:"posts"`
}
type userSimpleDTO struct {
	ID       int64  `json:"id"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
	Sex      int    `json:"sex"`
	DeptID   int64  `json:"deptId"`
	DeptName string `json:"deptName"`
}

func userDetail(u domain.AdminUser, dept domain.Dept) userDetailDTO {
	result := userDetailDTO{userDTO: userResponse(u), DeptName: dept.Name, Roles: []roleDTO{}, Posts: []postDTO{}}
	if dept.ID > 0 {
		dto := deptResponse(dept)
		result.Dept = &dto
	}
	return result
}

func (h *Handler) userDepartments(ctx context.Context, users []domain.AdminUser) (map[int64]domain.Dept, error) {
	ids := []int64{}
	for _, u := range users {
		if u.DeptID > 0 {
			ids = append(ids, u.DeptID)
		}
	}
	result := map[int64]domain.Dept{}
	if len(ids) == 0 {
		return result, nil
	}
	// Users have already passed their data filter; include the department name
	// for those users even when their role grants access only to themselves.
	page, err := h.service.ListDepts(ctxmeta.WithoutDataScope(ctx), app.Query{PageQuery: pagination.PageQuery{PageSize: -1}, IDs: domain.UniqueIDs(ids)})
	if err != nil {
		return nil, err
	}
	for _, d := range page.List {
		result[d.ID] = d
	}
	return result, nil
}
func (h *Handler) registerUsers(r *httpx.Router) {
	s := h.service
	for _, operation := range []string{"create", "update"} {
		method := http.MethodPost
		if operation == "update" {
			method = http.MethodPut
		}
		h.route(r, method, "user/"+operation, "system:user:"+operation, func(c *gin.Context) {
			var dto struct {
				userDTO
				Password string `json:"password"`
			}
			if c.ShouldBindJSON(&dto) != nil || operation == "update" && dto.ID <= 0 || operation == "create" && dto.ID != 0 {
				h.fail(c, domain.ErrInvalidArgument)
				return
			}
			id, err := s.SaveUser(c.Request.Context(), dto.entity(), dto.Password)
			if operation == "create" {
				h.complete(c, id, err)
			} else {
				h.done(c, err)
			}
		})
	}
	h.route(r, http.MethodDelete, "user/delete", "system:user:delete", func(c *gin.Context) {
		id, err := idParam(c, "id")
		if err != nil {
			h.fail(c, err)
			return
		}
		h.done(c, s.DeleteUsers(c.Request.Context(), []int64{id}))
	})
	h.route(r, http.MethodDelete, "user/delete-list", "system:user:delete", func(c *gin.Context) {
		ids, err := idsParam(c)
		if err != nil {
			h.fail(c, err)
			return
		}
		h.done(c, s.DeleteUsers(c.Request.Context(), ids))
	})
	h.route(r, http.MethodGet, "user/get", "system:user:query", func(c *gin.Context) {
		id, err := idParam(c, "id")
		if err != nil {
			h.fail(c, err)
			return
		}
		u, err := s.GetUser(c.Request.Context(), id)
		if err != nil {
			h.fail(c, err)
			return
		}
		depts, err := h.userDepartments(c.Request.Context(), []domain.AdminUser{u})
		h.complete(c, userDetail(u, depts[u.DeptID]), err)
	})
	h.route(r, http.MethodGet, "user/page", "system:user:query", func(c *gin.Context) {
		q, err := query(c)
		if err != nil {
			h.fail(c, err)
			return
		}
		page, err := s.PageUsers(c.Request.Context(), q)
		if err != nil {
			h.fail(c, err)
			return
		}
		depts, err := h.userDepartments(c.Request.Context(), page.List)
		result := mapPage(page, func(u domain.AdminUser) userDetailDTO { return userDetail(u, depts[u.DeptID]) })
		h.complete(c, result, err)
	})
	for _, alias := range []string{"list-all-simple", "simple-list"} {
		h.route(r, http.MethodGet, "user/"+alias, "", func(c *gin.Context) {
			status := 0
			page, err := s.PageUsers(c.Request.Context(), app.Query{PageQuery: pagination.PageQuery{PageSize: -1}, Status: &status})
			if err != nil {
				h.fail(c, err)
				return
			}
			depts, err := h.userDepartments(c.Request.Context(), page.List)
			h.complete(c, mapPage(page, func(u domain.AdminUser) userSimpleDTO {
				return userSimpleDTO{ID: u.ID, Nickname: u.Nickname, Avatar: u.Avatar, Sex: u.Sex, DeptID: u.DeptID, DeptName: depts[u.DeptID].Name}
			}).List, err)
		})
	}
	h.route(r, http.MethodPut, "user/update-password", "system:user:update-password", func(c *gin.Context) {
		var dto struct {
			ID       int64  `json:"id"`
			Password string `json:"password"`
		}
		if c.ShouldBindJSON(&dto) != nil || dto.ID <= 0 {
			h.fail(c, domain.ErrInvalidArgument)
			return
		}
		h.done(c, s.UpdatePassword(c.Request.Context(), dto.ID, dto.Password))
	})
	h.route(r, http.MethodPut, "user/update-status", "system:user:update", func(c *gin.Context) {
		var dto struct {
			ID     int64 `json:"id"`
			Status *int  `json:"status"`
		}
		if c.ShouldBindJSON(&dto) != nil || dto.ID <= 0 || dto.Status == nil {
			h.fail(c, domain.ErrInvalidArgument)
			return
		}
		h.done(c, s.UpdateUserStatus(c.Request.Context(), dto.ID, *dto.Status))
	})
	h.route(r, http.MethodGet, "user/profile/get", "", func(c *gin.Context) {
		u, err := s.Profile(c.Request.Context())
		if err != nil {
			h.fail(c, err)
			return
		}
		principal, _ := ctxmeta.User(c.Request.Context())
		ctx := ctxmeta.WithTenantID(ctxmeta.WithoutDataScope(c.Request.Context()), principal.TenantID)
		depts, err := h.userDepartments(ctx, []domain.AdminUser{u})
		if err != nil {
			h.fail(c, err)
			return
		}
		result := userDetail(u, depts[u.DeptID])
		ids, err := s.UserRoleIDs(ctx, u.ID)
		if err != nil {
			h.fail(c, err)
			return
		}
		for _, id := range ids {
			role, err := s.GetRole(ctx, id)
			if err != nil {
				h.fail(c, err)
				return
			}
			result.Roles = append(result.Roles, roleResponse(role))
		}
		for _, id := range u.PostIDs {
			post, err := s.GetPost(ctx, id)
			if err != nil {
				h.fail(c, err)
				return
			}
			result.Posts = append(result.Posts, postResponse(post))
		}
		httpx.Success(c, result)
	})
	h.route(r, http.MethodPut, "user/profile/update", "", func(c *gin.Context) {
		var dto struct {
			Nickname string `json:"nickname"`
			Email    string `json:"email"`
			Mobile   string `json:"mobile"`
			Sex      int    `json:"sex"`
		}
		if c.ShouldBindJSON(&dto) != nil {
			h.fail(c, domain.ErrInvalidArgument)
			return
		}
		h.done(c, s.UpdateProfile(c.Request.Context(), dto.Nickname, dto.Email, dto.Mobile, dto.Sex))
	})
	h.route(r, http.MethodPut, "user/profile/update-password", "", func(c *gin.Context) {
		var dto struct {
			OldPassword string `json:"oldPassword"`
			NewPassword string `json:"newPassword"`
		}
		if c.ShouldBindJSON(&dto) != nil {
			h.fail(c, domain.ErrInvalidArgument)
			return
		}
		h.done(c, s.UpdateOwnPassword(c.Request.Context(), dto.OldPassword, dto.NewPassword))
	})
	for _, method := range []string{http.MethodPost, http.MethodPut} {
		h.route(r, method, "user/profile/update-avatar", "", func(c *gin.Context) {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, (2<<20)+(64<<10))
			if err := c.Request.ParseMultipartForm(2 << 20); err != nil {
				h.fail(c, domain.ErrInvalidArgument)
				return
			}
			defer func() { _ = c.Request.MultipartForm.RemoveAll() }()
			file, header, err := c.Request.FormFile("avatarFile")
			if err != nil {
				file, header, err = c.Request.FormFile("file")
			}
			if err != nil {
				h.fail(c, domain.ErrInvalidArgument)
				return
			}
			defer func() { _ = file.Close() }()
			data, err := io.ReadAll(io.LimitReader(file, (2<<20)+1))
			if err != nil {
				h.fail(c, err)
				return
			}
			url, err := s.UpdateAvatar(c.Request.Context(), header.Filename, data)
			h.complete(c, url, err)
		})
	}
	h.registerUserExcel(r)
}
