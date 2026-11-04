# 测试分层与工程质量验收规范

> 对应《Go 项目工程目录结构设计规范 1.2》第 08 节与第 13 节。

## 一、四级测试分层体系（含真实 MySQL / Redis 集成测试）

为同时兼顾“本地秒级反馈”与“生产级真实依赖可靠性”，测试体系分为以下 4 层：

| 测试层级 | 存放位置 | 运行命令 | 依赖环境 | 核心覆盖目标 |
| :--- | :--- | :--- | :--- | :--- |
| **1. 架构守卫与领域单元测试（Arch & Unit Test）** | `cmd/api/arch_test.go`<br>`internal/<domain>/domain/*_test.go`<br>`internal/<domain>/app/*_test.go` | `make test` | 纯内存（零 DB/Redis） | • **AST 架构守卫**：强制校验 `domain` 仅导入标准库、`app` 不导入框架包<br>• **领域纯规则**：部门树防环、子孙部门展开、角色菜单差集与套餐回收算法、5 级数据权限合并、路径防穿越清洗 |
| **2. 快速契约测试（Fast Adapter Test）** | `internal/<domain>/adapter/*/*_test.go`<br>`internal/platform/*/*_test.go` | `make test` | SQLite 内存库 + `miniredis` 替身 | 快速验证仓储 CRUD、GORM 多租户自动隔离回调、5 级数据权限子句改写、软删除、审计字段填充、HTTP 路由状态码 |
| **3. 真实生产依赖集成测试（Real Dependency Integration Test）** | `tests/integration/*_test.go` | `make test-integration` | **真实 MySQL 8.0 + 真实 Redis 7**（通过 `OWOT_TEST_MYSQL_DSN` 与 `OWOT_TEST_REDIS_ADDR` 注入，配套 Docker Compose 与 CI Service Containers） | • 验证 `migrations/mysql/` DDL 与索引在真实 MySQL 上的执行与唯一键冲突行为<br>• 验证真实 MySQL 事务隔离、回滚与 `sys_outbox_event` 同事务原子提交<br>• 验证Outbox Relay 断线重放与权限缓存版本恢复<br>• 验证真实 Redis 上的 Lua 分布式锁互斥、防重放幂等与限流脚本 |
| **4. 端到端验收测试（E2E Test）** | `tests/e2e/*_test.go` | `make test` / `make test-integration` | 完整组装 `cmd/api` HTTP Server | 完整用户路径闭环（登录 → 权限菜单树 → 部门/岗位/用户 CRUD → 创建新租户级联初始化 → 跨租户隔离校验 → 套餐变更回收权限 → RBAC 403 越权拦截） |

---

## 二、标准 Makefile 校验入口（已落地）

根目录 [`Makefile`](../../Makefile) 提供本地与 CI 完全一致的校验目标：

```makefile
fmt:               # gofmt 格式化检查与修复
vet:               # go vet 静态分析
test:              # 运行单元、架构守卫、快速适配器与 E2E 测试
race:              # 开启数据竞争检测运行测试 (go test -race ./...)
lint:              # 运行 golangci-lint（若未安装则回退 gofmt -s -l + go vet）
build:             # 编译 ./cmd/api 到 bin/owot-server
check:             # 一键执行 fmt -> vet -> test -> race -> lint -> build
test-integration:  # 连接真实 MySQL + Redis 执行 tests/integration/...
```

---

## 三、工程验收清单（Checklist）

### 3.1 架构与目录（强制项验收）
- [x] 单 `go.mod` 结构，无 `pkg/`、`common/`、`utils/`、`shared/` 目录，无未使用的冗余依赖。
- [x] `make fmt vet test race lint build check` 全部可执行且通过。
- [x] AST 架构守卫单测（[`cmd/api/arch_test.go`](../../cmd/api/arch_test.go)）自动校验：
  - `internal/*/domain` 100% 仅导入 Go 标准库；
  - `internal/*/app` 仅导入标准库、同领域 `domain` 与纯标准库包 `internal/platform/{ctxmeta,pagination}`；
  - `internal/platform/*` 不导入任何业务领域包。

### 3.2 功能与安全（一期业务代码阶段验收）
- [ ] 空分页列表序列化为 `"list": []`，时间字段序列化为毫秒时间戳。
- [ ] 租户隔离：租户 A 无法读写租户 B 的用户/部门/角色；无租户上下文访问租户表被拦截。
- [ ] 租户级联：创建租户自动生成默认部门、`tenant_admin` 角色与管理员账号；缩减租户套餐菜单自动回收角色多余菜单权限。
- [ ] 权限缓存恢复：模拟广播失败后，通过 Outbox Relay 重放（`≤ 2s`）与本地 TTL（`≤ 30s`）确保已回收权限必然失效；强退/禁用账号 `0ms` 立即生效。
- [ ] 真实依赖验证：在真实 MySQL 8.0 + Redis 7 上通过 `make test-integration`。
