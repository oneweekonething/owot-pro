package http

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"oneweekonething.com/owot-pro/internal/platform/httpx"
	"oneweekonething.com/owot-pro/internal/system/app"
)

type Handler struct {
	dict   *app.DictService
	notice *app.NoticeService
	audit  *app.AuditLogService
	logger *slog.Logger
}

func NewHandler(dict *app.DictService, notice *app.NoticeService, audit *app.AuditLogService, logger *slog.Logger) *Handler {
	return &Handler{dict: dict, notice: notice, audit: audit, logger: logger}
}

func (h *Handler) Register(router *httpx.Router) {
	add := func(method, resource, action, permission string, handler gin.HandlerFunc) {
		route := httpx.Route{Permission: permission}
		if method != http.MethodGet {
			route.AuditType, route.AuditSubType = resource, action
		}
		router.Handle(method, "/admin-api/system/"+resource+"/"+action, route, handler)
		if action == "export" {
			router.Handle(method, "/admin-api/system/"+resource+"/export-excel", route, handler)
		}
	}
	for _, action := range []string{"create", "update", "delete", "delete-list", "get", "page", "export"} {
		method := http.MethodGet
		switch action {
		case "create":
			method = http.MethodPost
		case "update":
			method = http.MethodPut
		case "delete", "delete-list":
			method = http.MethodDelete
		}
		permission := action
		if action == "get" || action == "page" {
			permission = "query"
		}
		if action == "delete-list" {
			permission = "delete"
		}
		add(method, "dict-type", action, "system:dict:"+permission, h.typeAction(action))
		add(method, "dict-data", action, "system:dict:"+permission, h.dataAction(action))
	}
	for _, action := range []string{"list-all-simple", "simple-list"} {
		add(http.MethodGet, "dict-type", action, "", h.simpleTypes)
		add(http.MethodGet, "dict-data", action, "", h.simpleData)
	}
	add(http.MethodGet, "dict-data", "type", "", h.dataByType)
	router.Handle(http.MethodGet, "/app-api/system/dict-data/type", httpx.Route{PermitAll: true, TenantIgnore: true}, h.dataByType)
	for _, action := range []string{"create", "update", "delete", "get", "page"} {
		method := http.MethodGet
		switch action {
		case "create":
			method = http.MethodPost
		case "update":
			method = http.MethodPut
		case "delete":
			method = http.MethodDelete
		}
		permission := action
		if action == "get" || action == "page" {
			permission = "query"
		}
		add(method, "notice", action, "system:notice:"+permission, h.noticeAction(action))
	}
	for _, resource := range []string{"operate-log", "login-log"} {
		for _, action := range []string{"page", "export"} {
			permission := action
			if action == "page" {
				permission = "query"
			}
			add(http.MethodGet, resource, action, "system:"+resource+":"+permission, h.auditAction(resource, action))
		}
	}
}
