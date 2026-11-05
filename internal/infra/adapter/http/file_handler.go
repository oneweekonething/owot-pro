package http

import (
	"bytes"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"oneweekonething.com/owot-pro/internal/infra/app"
	"oneweekonething.com/owot-pro/internal/infra/domain"
	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
	"oneweekonething.com/owot-pro/internal/platform/httpx"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
	"oneweekonething.com/owot-pro/internal/platform/timeutil"
)

type FileHandler struct {
	service *app.FileService
	logger  *slog.Logger
}

func NewFileHandler(service *app.FileService, logger *slog.Logger) *FileHandler {
	return &FileHandler{service: service, logger: logger}
}

func (h *FileHandler) Register(router *httpx.Router) {
	const files = "/admin-api/infra/file/"
	const configs = "/admin-api/infra/file-config/"
	addConfig := func(method, action, permission string, handle gin.HandlerFunc) {
		route := httpx.Route{Permission: "infra:file-config:" + permission}
		if method != http.MethodGet || action == "test" {
			route.AuditType, route.AuditSubType = "infra-file-config", action
		}
		router.Handle(method, configs+action, route, func(c *gin.Context) {
			user, _ := ctxmeta.User(c.Request.Context())
			if user.TenantID != 1 || route.AuditType != "" && !user.SuperAdmin {
				httpx.Failure(c, 403, "无权管理全局文件存储配置")
				return
			}
			c.Header("Cache-Control", "no-store")
			handle(c)
		})
	}
	addConfig(http.MethodPost, "create", "create", h.saveConfig(true))
	addConfig(http.MethodPut, "update", "update", h.saveConfig(false))
	addConfig(http.MethodPut, "update-master", "update", h.updateMaster)
	addConfig(http.MethodDelete, "delete", "delete", h.remove(true, false))
	addConfig(http.MethodDelete, "delete-list", "delete", h.remove(true, true))
	addConfig(http.MethodGet, "get", "query", h.getConfig)
	addConfig(http.MethodGet, "page", "query", h.list(true))
	addConfig(http.MethodGet, "test", "query", h.testConfig)

	router.Handle(http.MethodPost, files+"upload", httpx.Route{}, h.upload)
	router.Handle(http.MethodGet, files+"presigned-url", httpx.Route{}, h.presign)
	router.Handle(http.MethodPost, files+"create", httpx.Route{}, h.complete)
	router.Handle(http.MethodGet, files+"page", httpx.Route{Permission: "infra:file:query"}, h.list(false))
	for _, action := range []string{"delete", "delete-list"} {
		router.Handle(http.MethodDelete, files+action, httpx.Route{Permission: "infra:file:delete",
			AuditType: "infra-file", AuditSubType: action}, h.remove(false, action == "delete-list"))
	}
	readRoute := httpx.Route{PermitAll: true, TenantIgnore: true}
	router.Handle(http.MethodGet, files+":configId/get/*path", readRoute, h.read)
	router.Handle(http.MethodHead, files+":configId/get/*path", readRoute, h.read)
}

type fileConfigInput struct {
	ID      int64               `json:"id"`
	Name    string              `json:"name"`
	Remark  string              `json:"remark"`
	Storage int                 `json:"storage"`
	Config  domain.FileSettings `json:"config"`
}

type fileConfigDTO struct {
	fileConfigInput
	Master     bool                `json:"master"`
	CreateTime timeutil.MillisTime `json:"createTime"`
}

const maskedAccessSecret = "********"

func fileConfigResponse(v domain.FileConfig, detail bool) fileConfigDTO {
	settings := v.Settings
	if detail && settings.AccessSecret != "" {
		settings.AccessSecret = maskedAccessSecret
	} else {
		settings.AccessSecret = ""
	}
	return fileConfigDTO{
		fileConfigInput: fileConfigInput{ID: v.ID, Name: v.Name, Remark: v.Remark, Storage: v.Storage, Config: settings},
		Master:          v.Master, CreateTime: timeutil.MillisTime{Time: v.CreateTime},
	}
}

func (h *FileHandler) saveConfig(create bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input fileConfigInput
		if !bindFileJSON(c, &input) {
			return
		}
		if input.Config.AccessSecret == maskedAccessSecret {
			if create {
				h.fail(c, domain.ErrInvalidArgument)
				return
			}
			input.Config.AccessSecret = ""
		}
		config := domain.FileConfig{ID: input.ID, Name: input.Name, Storage: input.Storage, Remark: input.Remark, Settings: input.Config}
		ctx := c.Request.Context()
		if create {
			id, err := h.service.CreateConfig(ctx, config)
			if err != nil {
				h.fail(c, err)
				return
			}
			httpx.SetAuditBizID(c, id)
			httpx.Success(c, id)
			return
		}
		if err := h.service.UpdateConfig(ctx, config); err != nil {
			h.fail(c, err)
			return
		}
		httpx.SetAuditBizID(c, config.ID)
		httpx.Success(c, true)
	}
}

func (h *FileHandler) getConfig(c *gin.Context) {
	id, err := configID(c.Query("id"))
	if err != nil {
		h.fail(c, err)
		return
	}
	config, err := h.service.GetConfig(c.Request.Context(), id)
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.Success(c, fileConfigResponse(config, true))
}

func (h *FileHandler) updateMaster(c *gin.Context) {
	id, err := configID(c.Query("id"))
	if err == nil {
		err = h.service.UpdateMaster(c.Request.Context(), id)
	}
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.SetAuditBizID(c, id)
	httpx.Success(c, true)
}

func (h *FileHandler) testConfig(c *gin.Context) {
	id, err := configID(c.Query("id"))
	if err != nil {
		h.fail(c, err)
		return
	}
	url, err := h.service.TestConfig(c.Request.Context(), id)
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.SetAuditBizID(c, id)
	httpx.Success(c, url)
}

func (h *FileHandler) remove(config, batch bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ids, err := requestIDs(c, batch)
		if err == nil {
			if config {
				err = h.service.DeleteConfigs(c.Request.Context(), ids)
			} else {
				err = h.service.Delete(c.Request.Context(), ids)
			}
		}
		if err != nil {
			h.fail(c, err)
			return
		}
		if len(ids) == 1 {
			httpx.SetAuditBizID(c, ids[0])
		}
		httpx.Success(c, true)
	}
}

func requestIDs(c *gin.Context, batch bool) ([]int64, error) {
	if !batch {
		id, err := configID(c.Query("id"))
		return []int64{id}, err
	}
	var ids []int64
	for _, value := range c.QueryArray("ids") {
		for _, raw := range strings.Split(value, ",") {
			id, err := configID(strings.TrimSpace(raw))
			if err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func (h *FileHandler) list(config bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		filter := app.FileFilter{Name: c.Query("name"), Path: c.Query("path"), Type: c.Query("type")}
		if err := c.ShouldBindQuery(&filter.PageQuery); err != nil {
			h.fail(c, domain.ErrInvalidArgument)
			return
		}
		var err error
		if raw := c.Query("storage"); config && raw != "" {
			filter.Storage, err = strconv.Atoi(raw)
			if err != nil {
				h.fail(c, domain.ErrInvalidArgument)
				return
			}
		}
		filter.From, filter.To, err = createTimeRange(c)
		if err != nil {
			h.fail(c, err)
			return
		}
		if config {
			result, err := h.service.PageConfigs(c.Request.Context(), filter)
			if err != nil {
				h.fail(c, err)
				return
			}
			rows := make([]fileConfigDTO, 0, len(result.List))
			for _, v := range result.List {
				rows = append(rows, fileConfigResponse(v, false))
			}
			httpx.Success(c, pagination.PageResult[fileConfigDTO]{List: rows, Total: result.Total})
			return
		}
		result, err := h.service.Page(c.Request.Context(), filter)
		if err != nil {
			h.fail(c, err)
			return
		}
		rows := make([]fileDTO, 0, len(result.List))
		for _, v := range result.List {
			rows = append(rows, fileDTO{ID: v.ID, ConfigID: v.ConfigID, Name: v.Name, Path: v.Path, URL: v.URL,
				Type: v.Type, Size: v.Size, CreateTime: timeutil.MillisTime{Time: v.CreateTime}})
		}
		httpx.Success(c, pagination.PageResult[fileDTO]{List: rows, Total: result.Total})
	}
}

type fileDTO struct {
	ID         int64               `json:"id"`
	ConfigID   int64               `json:"configId"`
	Name       string              `json:"name"`
	Path       string              `json:"path"`
	URL        string              `json:"url"`
	Type       string              `json:"type"`
	Size       int64               `json:"size"`
	CreateTime timeutil.MillisTime `json:"createTime"`
}

func (h *FileHandler) upload(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, domain.MaxFileSize+(1<<20))
	err := c.Request.ParseMultipartForm(1 << 20)
	if c.Request.MultipartForm != nil {
		defer func() { _ = c.Request.MultipartForm.RemoveAll() }()
	}
	if err != nil {
		h.fail(c, domain.ErrInvalidArgument)
		return
	}
	f, header, err := c.Request.FormFile("file")
	if err != nil {
		h.fail(c, domain.ErrInvalidArgument)
		return
	}
	defer func() { _ = f.Close() }()
	// Parse the original multipart filename before FormFile normalizes it.
	_, params, err := mime.ParseMediaType(header.Header.Get("Content-Disposition"))
	if err != nil || params["filename"] == "" {
		h.fail(c, domain.ErrInvalidArgument)
		return
	}
	data, err := io.ReadAll(io.LimitReader(f, domain.MaxFileSize+1))
	if err != nil {
		h.fail(c, err)
		return
	}
	url, err := h.service.Upload(c.Request.Context(), params["filename"], c.PostForm("directory"), data)
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.Success(c, url)
}

func (h *FileHandler) presign(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	result, err := h.service.Presign(c.Request.Context(), c.Query("name"), c.Query("directory"))
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.Success(c, result)
}

func (h *FileHandler) complete(c *gin.Context) {
	var input struct {
		ConfigID int64  `json:"configId"`
		Path     string `json:"path"`
	}
	if !bindFileJSON(c, &input) {
		return
	}
	id, err := h.service.CompleteUpload(c.Request.Context(), input.ConfigID, input.Path)
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.Success(c, id)
}

func bindFileJSON(c *gin.Context, input any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	if err := c.ShouldBindJSON(input); err != nil {
		httpx.Failure(c, 400, "请求参数不正确")
		return false
	}
	return true
}

func (h *FileHandler) read(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, err := configID(c.Param("configId"))
	if err != nil {
		h.fail(c, domain.ErrFileNotFound)
		return
	}
	file, data, err := h.service.Read(c.Request.Context(), id, strings.TrimPrefix(c.Param("path"), "/"), c.Query("download"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "sandbox")
	c.Header("Referrer-Policy", "no-referrer")
	contentType := http.DetectContentType(data)
	if !domain.InlineFileType(contentType) {
		c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": file.Name}))
		contentType = "application/octet-stream"
	}
	c.Header("Content-Type", contentType)
	http.ServeContent(c.Writer, c.Request, file.Name, file.CreateTime, bytes.NewReader(data))
}
