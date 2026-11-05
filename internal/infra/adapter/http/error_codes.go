package http

import (
	"errors"

	"github.com/gin-gonic/gin"

	"oneweekonething.com/owot-pro/internal/infra/domain"
	"oneweekonething.com/owot-pro/internal/platform/httpx"
)

func (h *ConfigHandler) fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidArgument):
		httpx.Failure(c, 400, "请求参数不正确")
	case errors.Is(err, domain.ErrConfigNotFound):
		httpx.Failure(c, 1_001_000_001, "参数配置不存在")
	case errors.Is(err, domain.ErrConfigKeyExists):
		httpx.Failure(c, 1_001_000_002, "参数配置 key 重复")
	case errors.Is(err, domain.ErrBuiltinConfigDelete):
		httpx.Failure(c, 1_001_000_003, "不能删除类型为系统内置的参数配置")
	case errors.Is(err, domain.ErrConfigHidden):
		httpx.Failure(c, 1_001_000_004, "不允许获取不可见配置")
	default:
		httpx.InternalError(c, h.logger, err)
	}
}

func (h *FileHandler) fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidArgument):
		httpx.Failure(c, 400, "请求参数不正确")
	case errors.Is(err, domain.ErrFileTooLarge):
		httpx.Failure(c, 400, "文件大小不能超过 20 MiB")
	case errors.Is(err, domain.ErrFileNotFound):
		httpx.Failure(c, 1_001_003_000, "文件不存在")
	case errors.Is(err, domain.ErrFileConfigNotFound):
		httpx.Failure(c, 1_001_002_000, "文件配置不存在")
	case errors.Is(err, domain.ErrFileMasterDelete):
		httpx.Failure(c, 1_001_002_001, "不能删除主文件配置")
	case errors.Is(err, domain.ErrFileConfigInUse):
		httpx.Failure(c, 409, "文件配置正在使用中")
	case errors.Is(err, domain.ErrFileDirectUpload):
		httpx.Failure(c, 400, "当前主文件配置不支持前端直传")
	default:
		httpx.InternalError(c, h.logger, err)
	}
}
