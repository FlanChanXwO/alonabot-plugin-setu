# 色图插件开发约定

本仓维护 AlonaBot 色图插件。入口是 `plugin.go` 的 `New`。SDK、协议与 Core 由 [AlonaBot Core](https://github.com/AlonaBot/AlonaBot) 维护。

| 任务 | 先读 | 完成前检查 |
| --- | --- | --- |
| 修改触发、配置或发送格式 | [行为契约](README.zh-CN.md)、[架构](ARCHITECTURE.md) | 检查真实 Core 入口和出站消息形态 |
| 修改图片验证、重试或并发 | [架构](ARCHITECTURE.md)、[开发指南](docs/development.md) | 检查坏链补齐、上游 5xx 和下载队列 |
| 编码、审查、文档或发布 | [文档入口](docs/index.md)、[贡献指南](CONTRIBUTING.zh-CN.md) | 格式化、相关验证、中英同步及发布授权 |

- 内部维护说明、注释和 commit message 使用简体中文；日志、错误与标识符使用英文；公开使用和贡献文档中英同步。用户消息直接使用中文。
- 编码规则跟随 [Core 编码规范](https://github.com/AlonaBot/AlonaBot/blob/main/docs/coding-standards.zh-CN.md)，维护方式参考 [OneBot 11 Adapter](https://github.com/AlonaBot/alonabot-adapter-onebot11)。
- 先读受影响的调用方与现有改动；Git 写操作在独立 worktree，提交、推送与发布按授权执行。新增依赖需经维护者批准。
- 提交覆盖真实边界的集成测试；一次性探针放入忽略的 `tmp/test/`。测试适配器的结果和真实平台联调分开记录。
