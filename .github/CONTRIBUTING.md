# 贡献指南 · Contributing

感谢你愿意改进 ChargingRecord Evolution。提交前请先阅读本页；完整的工程约定、领域不变量与发版流程见 [`AGENTS.md`](../AGENTS.md)。

## 报告问题 / Reporting issues

- 先搜索现有 issue，并确认问题在[最新 Release](https://github.com/f1owkang/CRecordEvolution/releases) 上仍可复现。
- 使用 [Bug report](https://github.com/f1owkang/CRecordEvolution/issues/new?template=bug_report.yml) 模板；一个 issue 只描述一个问题。
- 电池/估算类问题必须附「设置 → 复制诊断」的 `batteryd diag` 文本（已脱敏，无序列号/IMEI/MAC/账号），不要只发截图。
- 附上版本与通道（stable/ML）、root 管理器、机型、Android 版本、复现步骤与期望结果。
- 安全漏洞不要公开提交，见 [`SECURITY.md`](SECURITY.md)。

## 开发环境 / Development setup

- Go 版本以 `go.mod` 为准；Node/pnpm 版本由 `package.json` 固定（Node ≥ 20.19，pnpm 11.28.5）。
- SQLite 使用纯 Go 的 `modernc.org/sqlite`，无需 CGO，Windows 上可直接跑测试。
- 只发布 `android/arm64` 静态二进制；非 arm64 不在支持范围。

## 构建与测试 / Build and test

```sh
go test ./... -count=1
go vet ./...
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o bin/batteryd ./cmd/batteryd

pnpm install --frozen-lockfile
pnpm run verify   # 构建 → 离线扫描 → 50KB gzip 上限
pnpm run test:ui  # vitest 纯逻辑单测
pnpm run smoke    # 无头浏览器冒烟；改 webui/ 后至少跑一次
pnpm run format   # Prettier 写回
```

- Go 逻辑改动必须补/改测试，`go test ./...` 与 `go vet ./...` 全绿。
- WebUI 改动必须通过 `pnpm run verify`、`pnpm run test:ui`；涉及渲染/交互时再跑 `pnpm run smoke`。
- shell 改动至少 `bash -n customize.sh action.sh service.sh`。安装器、管理器 WebView、edge-to-edge 等无真机验证的路径，请在 PR 中注明「未在真机验证」并给出最小验证步骤。
- 产物有两条硬约束：完全离线（无 `fetch`/`XMLHttpRequest`/远程 URL/远程 import），gzip ≤ 50KB。

## 提交规范 / Commit style

- 格式：`type(scope): 中文单行主题`，冒号后一个空格，约 ≤ 40 汉字，不以句号结尾，不写 issue 编号。
- `type`：`feat`/`fix`/`docs`/`chore`/`add`/`refactor`/`ci`/`perf`；`scope` 用模块级小写词（`battery`/`webui`/`installer`/`release`/`docs`/`ci` 等）。
- 正文用 `-` 列表写 2~6 条：根因（现象 → 机理）、改动要点、测试/回归。
- 提交前先 `git log --format='%h %s' -10` 对齐既有风格。已推送到 `main` 的提交由维护者决定是否改写；PR 内可按需 rebase/squash。

## PR 要求 / Pull request requirements

- 从 `main` 切分支，一个 PR 只做一件事，保持可回退。
- 不要提交 `bin/`、`webroot/`、`node_modules/`、`artifacts/`、zip、`*.exe`、数据库或密钥/签名文件。
- 新增或修改 UI 文案时同时更新中英文；新增 JSON 字段时同步 Go 出口与 WebUI。
- 改版本号时同步 `module.prop` 与 `package.json`；版本号与发布由维护者执行。
- 新增运行时依赖必须进 bundle、更新 `THIRD_PARTY_NOTICES.txt`，且不得引入联网/遥测或与 GPL-2.0-only 不兼容的许可证。
- 设备端文件（`.sh`、`module.prop`、`META-INF/**`）必须保持 LF 换行。
- 提交 PR 即表示同意以 [GPL-2.0-only](../LICENSE) 授权你的贡献。
