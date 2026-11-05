package http

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"oneweekonething.com/owot-pro/internal/infra/app"
	"oneweekonething.com/owot-pro/internal/infra/domain"
	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
	"oneweekonething.com/owot-pro/internal/platform/excelx"
	"oneweekonething.com/owot-pro/internal/platform/httpx"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
	"oneweekonething.com/owot-pro/internal/platform/timeutil"
)

type ConfigHandler struct {
	service *app.ConfigService
	logger  *slog.Logger
}

func NewConfigHandler(service *app.ConfigService, logger *slog.Logger) *ConfigHandler {
	return &ConfigHandler{service: service, logger: logger}
}

func (h *ConfigHandler) Register(router *httpx.Router) {
	const admin = "/admin-api/infra/config/"
	add := func(method, action, permission string, handler gin.HandlerFunc) {
		route := httpx.Route{Permission: "infra:config:" + permission}
		if method != http.MethodGet {
			route.AuditType, route.AuditSubType = "infra-config", action
		}
		router.Handle(method, admin+action, route, func(c *gin.Context) {
			// Configuration is global and can contain private service values.
			// A visit-tenant header does not change the caller's home tenant.
			user, ok := ctxmeta.User(c.Request.Context())
			if !ok || user.TenantID != 1 {
				httpx.Failure(c, 403, "无权管理全局参数配置")
				return
			}
			c.Header("Cache-Control", "no-store")
			handler(c)
		})
	}
	add(http.MethodPost, "create", "create", h.save(true))
	add(http.MethodPut, "update", "update", h.save(false))
	add(http.MethodDelete, "delete", "delete", h.remove(false))
	add(http.MethodDelete, "delete-list", "delete", h.remove(true))
	add(http.MethodGet, "get", "query", h.get)
	add(http.MethodGet, "page", "query", h.list(false))
	add(http.MethodGet, "export", "export", h.list(true))
	add(http.MethodGet, "export-excel", "export", h.list(true))
	router.Handle(http.MethodGet, admin+"get-value-by-key", httpx.Route{}, h.value(false))
	router.Handle(http.MethodGet, "/app-api/infra/config/get-value-by-key", httpx.Route{PermitAll: true, TenantIgnore: true}, h.value(true))
}

type configInput struct {
	ID       int64  `json:"id"`
	Category string `json:"category"`
	Name     string `json:"name"`
	Key      string `json:"key"`
	Value    string `json:"value"`
	Visible  *bool  `json:"visible" binding:"required"`
	Remark   string `json:"remark"`
}

type configDTO struct {
	ID         int64               `json:"id"`
	Category   string              `json:"category"`
	Name       string              `json:"name"`
	Key        string              `json:"key"`
	Value      string              `json:"value"`
	Type       int                 `json:"type"`
	Visible    bool                `json:"visible"`
	Remark     string              `json:"remark"`
	CreateTime timeutil.MillisTime `json:"createTime"`
	UpdateTime timeutil.MillisTime `json:"updateTime"`
}

func configResponse(config domain.Config) configDTO {
	return configDTO{ID: config.ID, Category: config.Category, Name: config.Name, Key: config.Key,
		Value: config.Value, Type: config.Type, Visible: config.Visible, Remark: config.Remark,
		CreateTime: timeutil.MillisTime{Time: config.CreateTime}, UpdateTime: timeutil.MillisTime{Time: config.UpdateTime}}
}

func (h *ConfigHandler) save(create bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input configInput
		if err := c.ShouldBindJSON(&input); err != nil {
			h.fail(c, domain.ErrInvalidArgument)
			return
		}
		config := domain.Config{ID: input.ID, Category: input.Category, Name: input.Name, Key: input.Key,
			Value: input.Value, Visible: *input.Visible, Remark: input.Remark}
		ctx := c.Request.Context()
		if create {
			id, err := h.service.Create(ctx, config)
			if err != nil {
				h.fail(c, err)
				return
			}
			httpx.SetAuditBizID(c, id)
			httpx.Success(c, id)
			return
		}
		if err := h.service.Update(ctx, config); err != nil {
			h.fail(c, err)
			return
		}
		httpx.SetAuditBizID(c, config.ID)
		httpx.Success(c, true)
	}
}

func (h *ConfigHandler) remove(batch bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ids, err := requestIDs(c, batch)
		if err != nil {
			h.fail(c, err)
			return
		}
		if err := h.service.Delete(c.Request.Context(), ids); err != nil {
			h.fail(c, err)
			return
		}
		if len(ids) == 1 {
			httpx.SetAuditBizID(c, ids[0])
		}
		httpx.Success(c, true)
	}
}

func configID(raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, domain.ErrInvalidArgument
	}
	return id, nil
}

func (h *ConfigHandler) get(c *gin.Context) {
	id, err := configID(c.Query("id"))
	if err != nil {
		h.fail(c, err)
		return
	}
	config, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.Success(c, configResponse(config))
}

func (h *ConfigHandler) value(public bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		value, err := h.service.VisibleValue(c.Request.Context(), c.Query("key"))
		if errors.Is(err, domain.ErrConfigNotFound) || public && errors.Is(err, domain.ErrConfigHidden) {
			httpx.Success(c, nil)
			return
		}
		if err != nil {
			h.fail(c, err)
			return
		}
		httpx.Success(c, value)
	}
}

func (h *ConfigHandler) list(export bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		filter, err := configFilter(c)
		if err != nil {
			h.fail(c, err)
			return
		}
		if export {
			filter.PageSize = -1
		}
		result, err := h.service.Page(c.Request.Context(), filter)
		if err != nil {
			h.fail(c, err)
			return
		}
		if export {
			h.export(c, result.List)
			return
		}
		list := make([]configDTO, 0, len(result.List))
		for _, config := range result.List {
			list = append(list, configResponse(config))
		}
		httpx.Success(c, pagination.PageResult[configDTO]{List: list, Total: result.Total})
	}
}

func configFilter(c *gin.Context) (app.ConfigFilter, error) {
	filter := app.ConfigFilter{Name: c.Query("name"), Key: c.Query("key")}
	if err := c.ShouldBindQuery(&filter.PageQuery); err != nil {
		return filter, domain.ErrInvalidArgument
	}
	if raw := c.Query("type"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return filter, domain.ErrInvalidArgument
		}
		filter.Type = &value
	}
	var err error
	filter.From, filter.To, err = createTimeRange(c)
	return filter, err
}

func createTimeRange(c *gin.Context) (from, to *time.Time, err error) {
	repeated := c.QueryArray("createTime")
	if len(repeated) == 0 {
		repeated = c.QueryArray("createTime[]")
	}
	for i, key := range []string{"createTime[0]", "createTime[1]"} {
		raw := c.Query(key)
		if raw == "" && i < len(repeated) {
			raw = repeated[i]
		}
		if raw == "" {
			continue
		}
		value, err := timeutil.Parse(raw)
		if err != nil {
			return nil, nil, domain.ErrInvalidArgument
		}
		if i == 0 {
			from = &value
		} else {
			to = &value
		}
	}
	return from, to, nil
}

func (h *ConfigHandler) export(c *gin.Context, configs []domain.Config) {
	headers := []string{"参数编号", "参数分组", "参数名称", "参数键名", "参数键值", "参数类型", "是否可见", "备注", "创建时间"}
	rows := make([][]any, 0, len(configs))
	for _, config := range configs {
		rows = append(rows, []any{config.ID, config.Category, config.Name, config.Key, config.Value,
			config.Type, config.Visible, config.Remark, config.CreateTime.UnixMilli()})
	}
	var output bytes.Buffer
	if err := excelx.Write(c.Request.Context(), &output, headers, rows); err != nil {
		h.fail(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="configs.xlsx"`)
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", output.Bytes())
}
