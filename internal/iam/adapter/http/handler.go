// Package http adapts IAM use cases to the yudao administration API.
package http

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"oneweekonething.com/owot-pro/internal/iam/app"
	"oneweekonething.com/owot-pro/internal/iam/domain"
	"oneweekonething.com/owot-pro/internal/platform/excelx"
	"oneweekonething.com/owot-pro/internal/platform/httpx"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
	"oneweekonething.com/owot-pro/internal/platform/timeutil"
)

const prefix = "/admin-api/system/"

type Handler struct {
	service      *app.Service
	logger       *slog.Logger
	dictionaries app.DictTranslator
}

func NewHandler(service *app.Service, logger *slog.Logger, dictionaries app.DictTranslator) *Handler {
	return &Handler{service: service, logger: logger, dictionaries: dictionaries}
}
func (h *Handler) complete(c *gin.Context, value any, err error) {
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.Success(c, value)
}
func (h *Handler) done(c *gin.Context, err error) { h.complete(c, true, err) }
func (h *Handler) route(r *httpx.Router, method, path, permission string, fn gin.HandlerFunc) {
	route := httpx.Route{Permission: permission}
	if method != http.MethodGet {
		route.AuditType = "IAM"
		route.AuditSubType = path
	}
	r.Handle(method, prefix+path, route, fn)
}
func idParam(c *gin.Context, name string) (int64, error) {
	id, err := strconv.ParseInt(c.Query(name), 10, 64)
	if err != nil || id <= 0 {
		return 0, domain.ErrInvalidArgument
	}
	return id, nil
}
func idsParam(c *gin.Context) ([]int64, error) {
	ids := []int64{}
	raw := c.QueryArray("ids")
	if len(raw) == 0 {
		raw = c.QueryArray("ids[]")
	}
	for _, value := range raw {
		for _, item := range strings.Split(value, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(item), 10, 64)
			if err != nil || id <= 0 {
				return nil, domain.ErrInvalidArgument
			}
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 || len(ids) > 1000 {
		return nil, domain.ErrInvalidArgument
	}
	return domain.UniqueIDs(ids), nil
}
func query(c *gin.Context) (app.Query, error) {
	var q app.Query
	if err := c.ShouldBindQuery(&q.PageQuery); err != nil {
		return q, domain.ErrInvalidArgument
	}
	q.PageQuery = q.Normalize()
	q.Name, q.Code, q.Username, q.Mobile, q.ClientID = c.Query("name"), c.Query("code"), c.Query("username"), c.Query("mobile"), c.Query("clientId")
	if raw := c.Query("nickname"); raw != "" {
		q.Name = raw
	}
	if raw := c.Query("status"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || !domain.ValidStatus(v) {
			return q, domain.ErrInvalidArgument
		}
		q.Status = &v
	}
	for _, field := range []struct {
		Name   string
		Target **int64
	}{{"packageId", &q.PackageID}, {"userId", &q.UserID}} {
		if raw := c.Query(field.Name); raw != "" {
			v, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || v < 0 {
				return q, domain.ErrInvalidArgument
			}
			*field.Target = &v
		}
	}
	if c.Query("deptId") != "" {
		id, err := idParam(c, "deptId")
		if err != nil {
			return q, err
		}
		q.DeptIDs = []int64{id}
	}
	if raw := c.Query("userType"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 || v > 2 {
			return q, domain.ErrInvalidArgument
		}
		q.UserType = &v
	}
	rangeValues := c.QueryArray("createTime")
	if len(rangeValues) == 0 {
		rangeValues = c.QueryArray("createTime[]")
	}
	for i, target := range []string{"createTime[0]", "createTime[1]"} {
		raw := c.Query(target)
		if raw == "" && i < len(rangeValues) {
			raw = rangeValues[i]
		}
		if raw == "" {
			continue
		}
		value, err := timeutil.Parse(raw)
		if err != nil {
			return q, domain.ErrInvalidArgument
		}
		if i == 0 {
			q.From = &value
		} else {
			q.To = &value
		}
	}
	if q.From != nil && q.To != nil && q.From.After(*q.To) {
		return q, domain.ErrInvalidArgument
	}
	return q, nil
}
func mapPage[T, D any](p pagination.PageResult[T], convert func(T) D) pagination.PageResult[D] {
	list := make([]D, 0, len(p.List))
	for _, v := range p.List {
		list = append(list, convert(v))
	}
	return pagination.PageResult[D]{List: list, Total: p.Total}
}
func (h *Handler) export(c *gin.Context, name string, headers []string, rows [][]any) {
	var buffer bytes.Buffer
	if err := excelx.Write(c.Request.Context(), &buffer, headers, rows); err != nil {
		h.fail(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="`+name+`.xlsx"`)
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", buffer.Bytes())
}

// resource centralizes the identical transport contract of the management CRUDs;
// business validation and transactions remain in the typed use cases.
type resource[T any, D interface{ entity() T }] struct {
	Path                                        string
	Get                                         func(context.Context, int64) (T, error)
	List                                        func(context.Context, app.Query) (pagination.PageResult[T], error)
	Save                                        func(context.Context, T) (int64, error)
	Delete                                      func(context.Context, []int64) error
	Response                                    func(T) D
	ID                                          func(D) int64
	ListRoute, PageRoute, Batch, Simple, Export bool
	ExportRow                                   func(T) []any
}

func registerResource[T any, D interface{ entity() T }](h *Handler, r *httpx.Router, v resource[T, D]) {
	permission := "system:" + v.Path + ":"
	for _, operation := range []string{"create", "update"} {
		method := http.MethodPost
		if operation == "update" {
			method = http.MethodPut
		}
		h.route(r, method, v.Path+"/"+operation, permission+operation, func(c *gin.Context) {
			var dto D
			if c.ShouldBindJSON(&dto) != nil || operation == "update" && v.ID(dto) <= 0 || operation == "create" && v.ID(dto) != 0 {
				h.fail(c, domain.ErrInvalidArgument)
				return
			}
			id, err := v.Save(c.Request.Context(), dto.entity())
			if operation == "create" {
				h.complete(c, id, err)
			} else {
				h.done(c, err)
			}
		})
	}
	h.route(r, http.MethodGet, v.Path+"/get", permission+"query", func(c *gin.Context) {
		id, err := idParam(c, "id")
		if err != nil {
			h.fail(c, err)
			return
		}
		value, err := v.Get(c.Request.Context(), id)
		h.complete(c, v.Response(value), err)
	})
	h.route(r, http.MethodDelete, v.Path+"/delete", permission+"delete", func(c *gin.Context) {
		id, err := idParam(c, "id")
		if err != nil {
			h.fail(c, err)
			return
		}
		h.done(c, v.Delete(c.Request.Context(), []int64{id}))
	})
	if v.Batch {
		h.route(r, http.MethodDelete, v.Path+"/delete-list", permission+"delete", func(c *gin.Context) {
			ids, err := idsParam(c)
			if err != nil {
				h.fail(c, err)
				return
			}
			h.done(c, v.Delete(c.Request.Context(), ids))
		})
	}
	if v.PageRoute {
		h.route(r, http.MethodGet, v.Path+"/page", permission+"query", func(c *gin.Context) {
			q, err := query(c)
			if err != nil {
				h.fail(c, err)
				return
			}
			page, err := v.List(c.Request.Context(), q)
			h.complete(c, mapPage(page, v.Response), err)
		})
	}
	if v.ListRoute {
		h.route(r, http.MethodGet, v.Path+"/list", permission+"query", func(c *gin.Context) {
			q, err := query(c)
			if err != nil {
				h.fail(c, err)
				return
			}
			q.PageSize = -1
			page, err := v.List(c.Request.Context(), q)
			h.complete(c, mapPage(page, v.Response).List, err)
		})
	}
	if v.Simple {
		aliases := []string{"list-all-simple", "simple-list"}
		if v.Path == "tenant-package" {
			aliases = append(aliases, "get-simple-list")
		}
		for _, action := range aliases {
			h.route(r, http.MethodGet, v.Path+"/"+action, "", func(c *gin.Context) {
				status := 0
				page, err := v.List(c.Request.Context(), app.Query{PageQuery: pagination.PageQuery{PageSize: -1}, Status: &status})
				h.complete(c, mapPage(page, v.Response).List, err)
			})
		}
	}
	if v.Export {
		for _, alias := range []string{"export", "export-excel"} {
			h.route(r, http.MethodGet, v.Path+"/"+alias, permission+"export", func(c *gin.Context) {
				q, err := query(c)
				if err != nil {
					h.fail(c, err)
					return
				}
				q.PageSize = -1
				page, err := v.List(c.Request.Context(), q)
				if err != nil {
					h.fail(c, err)
					return
				}
				rows := [][]any{}
				for _, item := range page.List {
					rows = append(rows, v.ExportRow(item))
				}
				h.export(c, v.Path, []string{"编号", "名称", "标识", "状态"}, rows)
			})
		}
	}
}
