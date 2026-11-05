package http

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"oneweekonething.com/owot-pro/internal/iam/domain"
	"oneweekonething.com/owot-pro/internal/platform/httpx"
)

func (h *Handler) registerTenants(r *httpx.Router) {
	s := h.service
	for _, path := range []string{"get-id-by-name", "get-by-website", "simple-list"} {
		r.Handle(http.MethodGet, prefix+"tenant/"+path, httpx.Route{PermitAll: true, TenantIgnore: true}, func(c *gin.Context) {
			if path == "simple-list" {
				list, err := s.EnabledTenants(c.Request.Context())
				result := []gin.H{}
				for _, t := range list {
					result = append(result, gin.H{"id": t.ID, "name": t.Name})
				}
				h.complete(c, result, err)
				return
			}
			name, website := "", ""
			if path == "get-id-by-name" {
				name = c.Query("name")
			} else {
				website = c.Query("website")
			}
			t, err := s.ResolveTenant(c.Request.Context(), name, website)
			if path == "get-id-by-name" {
				h.complete(c, t.ID, err)
			} else {
				h.complete(c, gin.H{"id": t.ID, "name": t.Name}, err)
			}
		})
	}
	for _, operation := range []string{"create", "update"} {
		method := http.MethodPost
		if operation == "update" {
			method = http.MethodPut
		}
		h.route(r, method, "tenant/"+operation, "system:tenant:"+operation, func(c *gin.Context) {
			var dto struct {
				tenantDTO
				Username string `json:"username"`
				Password string `json:"password"`
			}
			if c.ShouldBindJSON(&dto) != nil || operation == "create" && dto.ID != 0 || operation == "update" && dto.ID <= 0 {
				h.fail(c, domain.ErrInvalidArgument)
				return
			}
			id, err := s.SaveTenant(c.Request.Context(), dto.entity(), dto.Username, dto.Password)
			if operation == "create" {
				h.complete(c, id, err)
			} else {
				h.done(c, err)
			}
		})
	}
	h.route(r, http.MethodGet, "tenant/get", "system:tenant:query", func(c *gin.Context) {
		id, err := idParam(c, "id")
		if err != nil {
			h.fail(c, err)
			return
		}
		t, err := s.GetTenant(c.Request.Context(), id)
		h.complete(c, tenantResponse(t), err)
	})
	h.route(r, http.MethodGet, "tenant/page", "system:tenant:query", func(c *gin.Context) {
		q, err := query(c)
		if err != nil {
			h.fail(c, err)
			return
		}
		page, err := s.PageTenants(c.Request.Context(), q)
		h.complete(c, mapPage(page, tenantResponse), err)
	})
	for _, path := range []string{"delete", "delete-list"} {
		h.route(r, http.MethodDelete, "tenant/"+path, "system:tenant:delete", func(c *gin.Context) {
			var ids []int64
			var err error
			if path == "delete" {
				var id int64
				id, err = idParam(c, "id")
				ids = []int64{id}
			} else {
				ids, err = idsParam(c)
			}
			if err != nil {
				h.fail(c, err)
				return
			}
			h.done(c, s.DeleteTenants(c.Request.Context(), ids))
		})
	}
	for _, alias := range []string{"export", "export-excel"} {
		h.route(r, http.MethodGet, "tenant/"+alias, "system:tenant:export", func(c *gin.Context) {
			q, err := query(c)
			if err != nil {
				h.fail(c, err)
				return
			}
			q.PageSize = -1
			page, err := s.PageTenants(c.Request.Context(), q)
			if err != nil {
				h.fail(c, err)
				return
			}
			rows := [][]any{}
			for _, t := range page.List {
				rows = append(rows, []any{t.ID, t.Name, t.ContactName, t.ContactMobile, t.Status, t.AccountCount, t.ExpireTime.UnixMilli()})
			}
			h.export(c, "tenants", []string{"编号", "租户名称", "联系人", "联系电话", "状态", "账号配额", "到期时间"}, rows)
		})
	}
}
