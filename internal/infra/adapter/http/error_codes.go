package http

import (
	"errors"
	"fmt"

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

func (h *OperationsHandler) fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidArgument):
		httpx.Failure(c, 400, "请求参数不正确")
	case errors.Is(err, domain.ErrJobNotFound):
		httpx.Failure(c, 1_001_001_000, "定时任务不存在")
	case errors.Is(err, domain.ErrJobHandlerUsed):
		httpx.Failure(c, 1_001_001_001, "任务处理器已被使用")
	case errors.Is(err, domain.ErrJobCron):
		httpx.Failure(c, 1_001_001_005, "Cron 表达式无效或没有后续执行时间")
	case errors.Is(err, domain.ErrJobHandlerMissing):
		httpx.Failure(c, 1_001_001_006, "任务处理器未注册")
	case errors.Is(err, domain.ErrJobChanged):
		httpx.Failure(c, 409, "任务已发生变更，请刷新后重试")
	case errors.Is(err, domain.ErrJobLogNotFound):
		httpx.Failure(c, 1_001_001_008, "任务日志不存在")
	case errors.Is(err, domain.ErrLogExportLimit):
		httpx.Failure(c, 400, fmt.Sprintf("单次最多导出 %d 条日志，请缩小查询范围", domain.MaxLogExportRows))
	case errors.Is(err, domain.ErrAccessLogNotFound):
		httpx.Failure(c, 1_001_005_000, "访问日志不存在")
	case errors.Is(err, domain.ErrErrorLogNotFound):
		httpx.Failure(c, 1_001_004_000, "错误日志不存在")
	case errors.Is(err, domain.ErrErrorLogHandled):
		httpx.Failure(c, 1_001_004_001, "错误日志已处理")
	default:
		httpx.InternalError(c, h.logger, err)
	}
}
