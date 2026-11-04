package http

import (
	"bytes"

	"github.com/gin-gonic/gin"

	"oneweekonething.com/owot-pro/internal/platform/excelx"
	"oneweekonething.com/owot-pro/internal/platform/httpx"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
	"oneweekonething.com/owot-pro/internal/system/app"
	"oneweekonething.com/owot-pro/internal/system/domain"
)

type dictTypeInput struct {
	ID     int64  `json:"id"`
	Name   string `json:"name" binding:"required"`
	Type   string `json:"type"`
	Status *int   `json:"status" binding:"required"`
	Remark string `json:"remark"`
}

type dictDataInput struct {
	ID        int64  `json:"id"`
	Sort      int    `json:"sort"`
	Label     string `json:"label" binding:"required"`
	Value     string `json:"value" binding:"required"`
	DictType  string `json:"dictType" binding:"required"`
	Status    *int   `json:"status" binding:"required"`
	ColorType string `json:"colorType"`
	CSSClass  string `json:"cssClass"`
	Remark    string `json:"remark"`
}

func (h *Handler) typeAction(action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		switch action {
		case "create", "update":
			var input dictTypeInput
			if err := c.ShouldBindJSON(&input); err != nil {
				h.fail(c, domain.ErrInvalidArgument)
				return
			}
			value := domain.DictType{ID: input.ID, Name: input.Name, Type: input.Type, Status: *input.Status, Remark: input.Remark}
			if action == "create" {
				id, err := h.dict.CreateType(ctx, value)
				if err != nil {
					h.fail(c, err)
					return
				}
				httpx.SetAuditBizID(c, id)
				httpx.Success(c, id)
			} else {
				if err := h.dict.UpdateType(ctx, value); err != nil {
					h.fail(c, err)
					return
				}
				httpx.SetAuditBizID(c, value.ID)
				httpx.Success(c, true)
			}
		case "get":
			id, err := positiveID(c)
			if err != nil {
				h.fail(c, err)
				return
			}
			value, err := h.dict.GetType(ctx, id)
			if err != nil {
				h.fail(c, err)
				return
			}
			httpx.Success(c, typeDTO(value))
		case "delete", "delete-list":
			ids, err := deleteIDs(c, action)
			if err == nil {
				err = h.dict.DeleteTypes(ctx, ids)
			}
			if err != nil {
				h.fail(c, err)
				return
			}
			if len(ids) == 1 {
				httpx.SetAuditBizID(c, ids[0])
			}
			httpx.Success(c, true)
		case "page", "export":
			filter, err := typeFilter(c)
			if err != nil {
				h.fail(c, err)
				return
			}
			if action == "export" {
				filter.PageSize = -1
			}
			result, err := h.dict.PageTypes(ctx, filter)
			if err != nil {
				h.fail(c, err)
				return
			}
			if action == "export" {
				rows := make([][]any, 0, len(result.List))
				for _, v := range result.List {
					rows = append(rows, []any{v.ID, v.Name, v.Type, statusLabel(v.Status), v.Remark, v.CreateTime.UnixMilli()})
				}
				h.export(c, "dict-types.xlsx", []string{"字典编号", "字典名称", "字典类型", "状态", "备注", "创建时间"}, rows)
				return
			}
			httpx.Success(c, pagination.PageResult[dictTypeDTO]{List: convert(result.List, typeDTO), Total: result.Total})
		}
	}
}

func (h *Handler) dataAction(action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		switch action {
		case "create", "update":
			var input dictDataInput
			if err := c.ShouldBindJSON(&input); err != nil {
				h.fail(c, domain.ErrInvalidArgument)
				return
			}
			value := domain.DictData{ID: input.ID, Sort: input.Sort, Label: input.Label, Value: input.Value,
				DictType: input.DictType, Status: *input.Status, ColorType: input.ColorType, CSSClass: input.CSSClass, Remark: input.Remark}
			if action == "create" {
				id, err := h.dict.CreateData(ctx, value)
				if err != nil {
					h.fail(c, err)
					return
				}
				httpx.SetAuditBizID(c, id)
				httpx.Success(c, id)
			} else {
				if err := h.dict.UpdateData(ctx, value); err != nil {
					h.fail(c, err)
					return
				}
				httpx.SetAuditBizID(c, value.ID)
				httpx.Success(c, true)
			}
		case "get":
			id, err := positiveID(c)
			if err != nil {
				h.fail(c, err)
				return
			}
			value, err := h.dict.GetData(ctx, id)
			if err != nil {
				h.fail(c, err)
				return
			}
			httpx.Success(c, dataDTO(value))
		case "delete", "delete-list":
			ids, err := deleteIDs(c, action)
			if err == nil {
				err = h.dict.DeleteData(ctx, ids)
			}
			if err != nil {
				h.fail(c, err)
				return
			}
			if len(ids) == 1 {
				httpx.SetAuditBizID(c, ids[0])
			}
			httpx.Success(c, true)
		case "page", "export":
			filter, err := dataFilter(c)
			if err != nil {
				h.fail(c, err)
				return
			}
			if action == "export" {
				filter.PageSize = -1
			}
			result, err := h.dict.PageData(ctx, filter)
			if err != nil {
				h.fail(c, err)
				return
			}
			if action == "export" {
				rows := make([][]any, 0, len(result.List))
				for _, v := range result.List {
					rows = append(rows, []any{v.ID, v.Label, v.Value, v.DictType, v.Sort, statusLabel(v.Status), v.Remark, v.CreateTime.UnixMilli()})
				}
				h.export(c, "dict-data.xlsx", []string{"字典编号", "字典标签", "字典键值", "字典类型", "排序", "状态", "备注", "创建时间"}, rows)
				return
			}
			httpx.Success(c, pagination.PageResult[dictDataDTO]{List: convert(result.List, dataDTO), Total: result.Total})
		}
	}
}

func (h *Handler) simpleTypes(c *gin.Context) {
	result, err := h.dict.PageTypes(c.Request.Context(), app.DictTypeFilter{PageQuery: pagination.PageQuery{PageSize: -1}})
	if err != nil {
		h.fail(c, err)
		return
	}
	type simple struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
		Type string `json:"type"`
	}
	httpx.Success(c, convert(result.List, func(v domain.DictType) simple { return simple{v.ID, v.Name, v.Type} }))
}

func (h *Handler) simpleData(c *gin.Context) {
	result, err := h.dict.SimpleData(c.Request.Context())
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.Success(c, convert(result, dataDTO))
}

func (h *Handler) dataByType(c *gin.Context) {
	values, err := h.dict.EnabledData(c.Request.Context(), c.Query("type"))
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.Success(c, convert(values, dataDTO))
}

func (h *Handler) export(c *gin.Context, name string, headers []string, rows [][]any) {
	var buffer bytes.Buffer
	if err := excelx.Write(c.Request.Context(), &buffer, headers, rows); err != nil {
		h.fail(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="`+name+`"`)
	c.Data(200, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", buffer.Bytes())
}

func deleteIDs(c *gin.Context, action string) ([]int64, error) {
	if action == "delete-list" {
		return idList(c)
	}
	id, err := positiveID(c)
	return []int64{id}, err
}

func statusLabel(status int) string {
	if status == domain.StatusEnabled {
		return "开启"
	}
	return "关闭"
}
