# IAM 领域局部规则（`internal/iam`）

> 继承根 `/AGENTS.md`，以下为 `internal/iam` 领域的局部增量约束：

1. **100% 纯标准库领域层**：
   - `internal/iam/domain/` **只允许导入 Go 标准库**，严禁导入 GORM、Gin、Redis、`bcrypt` 或任何 `internal/platform/*` 包。
   - 密码明文格式校验、部门树防环、子孙部门展开、角色菜单差集、套餐变更权限回收算法全部写在 `domain/` 中并配齐纯内存单测。
   - 密码哈希（`bcrypt`）通过 `app/ports.go` 中的消费方接口 `PasswordHasher` 调用 `adapter/security` 实现；数值错误码映射在 `adapter/http` 完成。
2. **内置对象保护**：
   - `super_admin` 与 `tenant_admin` 角色（`type=1`）禁止删除或修改 `code`。
   - 系统租户（`package_id=0`）与 `admin` 初始管理员禁止删除或禁用。
   - `default` OAuth2 客户端禁止删除。
3. **租户上下文切换**：在创建新租户级联初始化部门/角色/管理员，或变更租户套餐同步角色菜单时，使用纯标准库包 `ctxmeta.WithTenantID(ctx, targetTenantID)` 切换到目标租户上下文执行。
4. **权限缓存一致性与失败恢复**：
   - 角色、菜单、角色菜单关联、用户角色关联、部门或租户套餐变更时，必须在同一数据库事务中写入 `sys_outbox_event`，由事务后广播 + Outbox Relay 重放（`≤ 2s`）+ 租户权限版本号 + 本地缓存 30s TTL 保证权限回收可靠生效。
   - 禁用用户或强退 Token 必须同步删除 Redis `oauth2_access_token:%s`，确保 `0ms` 立即生效。
5. **详细规格**：见 [`docs/domains/IAM.md`](../../docs/domains/IAM.md)。
