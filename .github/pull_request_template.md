<!-- 感谢提交 PR。请先阅读 .github/CONTRIBUTING.md 与 AGENTS.md。 -->

## 变更说明 / Summary

<!-- 做了什么、为什么；关联 issue 可写 "Fixes #123" -->

## 类型 / Type

- [ ] fix
- [ ] feat
- [ ] docs
- [ ] chore / refactor / perf / ci

## 验证 / Verification

- [ ] Go：`go test ./... -count=1`、`go vet ./...` 通过
- [ ] WebUI：`pnpm run verify`、`pnpm run test:ui` 通过（改 `webui/` 时）
- [ ] shell：`bash -n customize.sh action.sh service.sh` 通过（改 `.sh` 时）
- [ ] 真机验证：涉及安装器/管理器 WebView/edge-to-edge 时说明机型与结果；无法验证请注明「未在真机验证」并给出最小验证步骤

## 检查清单 / Checklist

- [ ] 提交信息符合 `type(scope): 中文主题`，正文写根因、改动与测试
- [ ] 未提交 `bin/`、`webroot/`、`node_modules/`、`artifacts/`、zip、`*.exe` 等产物
- [ ] 新增/修改 UI 文案已同时更新中英文；新增 JSON 字段已同步 Go 与 WebUI
- [ ] 改版本号时已同步 `module.prop` 与 `package.json`
- [ ] 新增运行时依赖已更新 `THIRD_PARTY_NOTICES.txt`，且未引入联网/遥测或不兼容许可证
- [ ] 设备端文件（`.sh`、`module.prop`、`META-INF/**`）保持 LF
- [ ] 已阅读 [CONTRIBUTING.md](https://github.com/f1owkang/CRecordEvolution/blob/main/.github/CONTRIBUTING.md) 与 [AGENTS.md](https://github.com/f1owkang/CRecordEvolution/blob/main/AGENTS.md)
