// Package http adapts System use cases to the yudao administration API.
package http

import (
	"errors"

	"github.com/gin-gonic/gin"

	"oneweekonething.com/owot-pro/internal/platform/httpx"
	"oneweekonething.com/owot-pro/internal/system/domain"
)

func (h *Handler) fail(c *gin.Context, err error) {
	codes := []struct {
		err  error
		code int
		msg  string
	}{
		{domain.ErrInvalidArgument, 400, "请求参数不正确"},
		{domain.ErrTenantRequired, 400, "缺少租户编号"},
		{domain.ErrDictTypeNotFound, 1_002_006_000, "字典类型不存在"},
		{domain.ErrDictTypeExists, 1_002_006_001, "字典类型已经存在"},
		{domain.ErrDictTypeImmutable, 1_002_006_002, "字典类型标识不可修改"},
		{domain.ErrDictTypeHasData, 1_002_006_003, "字典类型包含数据项，无法删除"},
		{domain.ErrDictDataNotFound, 1_002_006_004, "字典数据不存在"},
		{domain.ErrDictValueExists, 1_002_006_005, "字典数据值已经存在"},
		{domain.ErrDictLabelNotFound, 1_002_006_006, "字典标签或值不存在"},
		{domain.ErrNoticeNotFound, 1_002_008_000, "通知公告不存在"},
	}
	for _, mapping := range codes {
		if errors.Is(err, mapping.err) {
			httpx.Failure(c, mapping.code, mapping.msg)
			return
		}
	}
	httpx.InternalError(c, h.logger, err)
}
