# OWOT-Pro (`oneweekonething.com/owot-pro`)

> **One Week, One Module; One Week, One System.**
> 基于《Go 项目工程目录结构设计规范 1.2》构建的模块化单体（Modular Monolith）企业级后端系统，对标 `ruoyi-vue-pro`，前端兼容 `yudao-ui-admin-vue3`。

## 快速导航
- **AI 与开发者全局规则**：[AGENTS.md](AGENTS.md)
- **工程设计文档总览**：[docs/README.md](docs/README.md)
- **System 模块启动与接入**：[docs/development/SYSTEM_IMPLEMENTATION.md](docs/development/SYSTEM_IMPLEMENTATION.md)
- **IAM 模块启动与接入**：[docs/development/IAM_IMPLEMENTATION.md](docs/development/IAM_IMPLEMENTATION.md)
- **架构与目录规范**：[docs/architecture/ARCHITECTURE.md](docs/architecture/ARCHITECTURE.md)
- **平台层设计规范**：[docs/architecture/PLATFORM.md](docs/architecture/PLATFORM.md)
- **业务领域设计**：
  - [IAM 领域（认证/用户/部门/岗位/RBAC/多租户/OAuth2）](docs/domains/IAM.md)
  - [System 领域（字典/公告/操作与登录日志）](docs/domains/SYSTEM.md)
  - [Infra 领域（参数配置/文件存储/定时任务/API 日志/Redis 监控）](docs/domains/INFRA.md)

## 标准校验命令
```bash
make fmt               # 格式化代码
make vet               # 静态检查
make test              # 运行单元测试与 AST 架构边界守卫测试
make race              # 数据竞争检测测试
make lint              # 代码规范与依赖方向检查
make build             # 编译 ./cmd/api -> bin/owot-server
make check             # 一键执行全部门禁 (fmt + vet + test + race + lint + build)
make test-integration  # 连接真实 MySQL + Redis 运行集成测试
```
