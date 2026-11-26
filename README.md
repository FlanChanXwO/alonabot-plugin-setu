# AlonaBot Setu Plugin

[简体中文](README.zh-CN.md)

Send “来份色图” in chat for one random non-R18 illustration at its original size. Use “来1份色图”, “来二份色图”, or “来三份色图” for a specific count, from 1 to 10. The Chinese form “两” also means two. These phrases replace `/setu`.

## Set the per-request limit

`max_pictures` defaults to `10` and accepts integers from 1 to 10. Set it to `3`, for example, to allow “来三份色图”. “来四份色图” reports the limit without contacting the image source. This setting applies to every adapter and delivery mode.

## Choose how OneBot 11 sends pictures

Set `forward_mode` in the AlonaBot plugin configuration:

| Value | OneBot 11 delivery |
| --- | --- |
| `off` (default) | One image chain |
| `multi` | Direct for one image; merged forward for several |
| `always` | Merged forward even for one image |

Other adapters always receive one image chain. The plugin validates originals before sending and requests only the missing images, for up to two replacement rounds. An upstream 5xx ends the request immediately. If delivery fails, the plugin replies once in Chinese.

## Test and package locally

Use the SDK and matching CLI specified by `go.mod`. Run `alonabot run .` and enter “来份色图” in the console. This accesses the live image source; see the [development guide](docs/development.md) for offline checks.

For Linux amd64, run `alonabot pack . --target linux/amd64`. See the [contribution guide](CONTRIBUTING.md#packaging-and-release) for the release workflow.
