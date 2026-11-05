package http

import (
	"errors"

	"github.com/gin-gonic/gin"

	"oneweekonething.com/owot-pro/internal/iam/domain"
	"oneweekonething.com/owot-pro/internal/platform/httpx"
)

func (h *Handler) fail(c *gin.Context, err error) {
	for _, m := range []struct {
		Err     error
		Code    int
		Message string
	}{
		{domain.ErrInvalidArgument, 400, "请求参数不正确"}, {domain.ErrForbidden, 403, "没有该操作权限"},
		{domain.ErrCredentials, 1_002_000_000, "账号或密码不正确"}, {domain.ErrDisabled, 1_002_000_001, "账号已被禁用"}, {domain.ErrTokenInvalid, 401, "令牌无效或已过期"},
		{domain.ErrUserNotFound, 1_002_003_000, "用户不存在"}, {domain.ErrDuplicate, 1_002_003_001, "账号、联系方式或业务标识已经存在"}, {domain.ErrPassword, 1_002_003_004, "密码须为4至16位且不包含空白字符"},
		{domain.ErrRoleNotFound, 1_002_001_000, "角色不存在"}, {domain.ErrMenuNotFound, 1_002_002_000, "菜单不存在"}, {domain.ErrDeptNotFound, 1_002_004_000, "部门不存在"}, {domain.ErrPostNotFound, 1_002_005_000, "岗位不存在"},
		{domain.ErrTenantNotFound, 1_002_015_000, "租户不存在"}, {domain.ErrTenantUnavailable, 1_002_015_001, "租户或套餐已禁用或过期"}, {domain.ErrQuotaExceeded, 1_002_015_002, "超出租户账号配额"}, {domain.ErrPackageNotFound, 1_002_016_000, "租户套餐不存在"},
		{domain.ErrClientNotFound, 1_002_020_000, "OAuth2客户端不存在或已禁用"}, {domain.ErrProtected, 1_002_001_001, "内置对象不允许此操作"}, {domain.ErrTreeCycle, 1_002_004_002, "父节点不能是自身或子孙节点"}, {domain.ErrHasChildren, 1_002_004_003, "对象仍被引用，无法删除或修改"},
	} {
		if errors.Is(err, m.Err) {
			httpx.Failure(c, m.Code, m.Message)
			return
		}
	}
	httpx.InternalError(c, h.logger, err)
}
