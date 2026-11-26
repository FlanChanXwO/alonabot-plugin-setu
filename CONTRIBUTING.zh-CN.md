# 贡献指南

[English](CONTRIBUTING.md)

先从 [AGENTS](AGENTS.md) 找到受影响的契约和验证入口。行为变化同步更新中英 README；测试覆盖实际 SDK 入口。提交 PR 时填写[模板](.github/PULL_REQUEST_TEMPLATE.md)，说明影响、复现和验证结果。诊断日志应移除凭据与私人数据。

## 打包与发布

维护者获准发布后，更新 `plugin.yaml` 版本和 `CHANGELOG.md` 对应的英文版本段，并使用匹配的版本标签触发模板发布工作流。标签发布前先通过[开发检查](docs/development.md)。本地只打包时用 `alonabot pack . --target linux/amd64`。
