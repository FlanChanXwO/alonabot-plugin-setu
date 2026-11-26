# 开发与验收

使用 `go.mod` 声明的 Go 和 AlonaBot SDK；CLI 用相同 SDK 版本构建。每次发布前核对最新稳定版 SDK，将已验证的版本记录在 `go.mod` 与 `go.sum`。CI 的安装命令见 [Component quality](../.github/workflows/ci.yml)。在仓库根目录运行：

```fish
go vet ./...
go test -race ./...
alonabot check .
alonabot test .
```

| 规则与来源 | 负责范围 | 检查与触发 |
| --- | --- | --- |
| 正则、数量及发送格式：[使用说明](../README.zh-CN.md) | 路由与发送 | `go test -race ./...`，CI 有 Go 改动时运行；真实 Core、测试 Adapter |
| 链接验证、缺图补齐及并发上限：[架构](../ARCHITECTURE.md) | 图源 | 同一测试入口使用离线 HTTP fixture，观察请求、退出条件和出站结果 |
| 组件配置：[Schema](../plugin_config_schema.json)、SDK 注册契约 | 插件入口及资源 | `alonabot check .` 和 `alonabot test .`，CI 有组件改动时运行；覆盖校验和离线激活 |
| 文档路由与依赖方向：[AGENTS](../AGENTS.md)、[架构](../ARCHITECTURE.md) | 维护者 | 修改时审查链接、导入和中英用法；编译检查依赖环，其余由审查负责 |

复现：`go test -run TestFailuresThroughCore -v`。坏链场景应只请求缺少的张数；503 场景只访问目录一次，用户收到一次中文提示。先看失败日志中的阶段和原始错误，再检查目录、原图响应或平台发送。真实 OneBot 11 的群聊和私聊转发需要单独测试会话；离线结果只覆盖 SDK 出站契约。
