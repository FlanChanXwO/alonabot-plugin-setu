# Contributing

[简体中文](CONTRIBUTING.zh-CN.md)

Start with [AGENTS](AGENTS.md) to find the relevant contracts and checks. Update both READMEs for behavior changes and test through the SDK entry point. Fill out the [PR template](.github/PULL_REQUEST_TEMPLATE.md) with impact, reproduction steps, and verification. Remove credentials and private data from logs.

## Packaging and release

With release authorization, update the version in `plugin.yaml` and its English section in `CHANGELOG.md`. Run the [development checks](docs/development.md) before using the matching version tag to trigger the template release workflow. For a local package only, run `alonabot pack . --target linux/amd64`.
