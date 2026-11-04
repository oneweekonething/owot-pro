package http

import (
	"github.com/gin-gonic/gin"

	"oneweekonething.com/owot-pro/internal/platform/httpx"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
	"oneweekonething.com/owot-pro/internal/system/app"
	"oneweekonething.com/owot-pro/internal/system/domain"
)

type noticeInput struct {
	ID      int64  `json:"id"`
	Title   string `json:"title" binding:"required"`
	Type    int    `json:"type"`
	Content string `json:"content" binding:"required"`
	Status  *int   `json:"status" binding:"required"`
}

func (h *Handler) noticeAction(action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		switch action {
		case "create", "update":
			var input noticeInput
			if err := c.ShouldBindJSON(&input); err != nil {
				h.fail(c, domain.ErrInvalidArgument)
				return
			}
			value := domain.Notice{ID: input.ID, Title: input.Title, Type: input.Type, Content: input.Content, Status: *input.Status}
			if action == "create" {
				id, err := h.notice.Create(ctx, value)
				if err != nil {
					h.fail(c, err)
					return
				}
				httpx.SetAuditBizID(c, id)
				httpx.Success(c, id)
			} else {
				if err := h.notice.Update(ctx, value); err != nil {
					h.fail(c, err)
					return
				}
				httpx.SetAuditBizID(c, value.ID)
				httpx.Success(c, true)
			}
		case "get", "delete":
			id, err := positiveID(c)
			if err != nil {
				h.fail(c, err)
				return
			}
			if action == "delete" {
				if err := h.notice.Delete(ctx, id); err != nil {
					h.fail(c, err)
					return
				}
				httpx.SetAuditBizID(c, id)
				httpx.Success(c, true)
			} else {
				value, err := h.notice.Get(ctx, id)
				if err != nil {
					h.fail(c, err)
					return
				}
				httpx.Success(c, toNoticeDTO(value))
			}
		case "page":
			filter := app.NoticeFilter{Title: c.Query("title")}
			var err error
			filter.PageQuery, err = pageQuery(c)
			if err == nil {
				filter.Status, err = optionalInt(c, "status")
			}
			if err == nil {
				filter.Type, err = optionalInt(c, "type")
			}
			if err != nil {
				h.fail(c, err)
				return
			}
			result, err := h.notice.Page(ctx, filter)
			if err != nil {
				h.fail(c, err)
				return
			}
			httpx.Success(c, pagination.PageResult[noticeDTO]{List: convert(result.List, toNoticeDTO), Total: result.Total})
		}
	}
}
