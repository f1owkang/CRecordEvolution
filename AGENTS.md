# AGENTS.md

本文件面向在本仓库工作的编码代理，说明项目目标、构建/测试方式、代码约定、领域不变量和发布流程。面向用户的介绍、安装步骤和功能说明在 [README.md](README.md) / [README_ZH.md](README_ZH.md)，不在这里重复。

## 项目概览

ChargingRecord Evolution 是一个 Android 模块（id=`CRecordEvolution`），用 Go 二进制 `batteryd` 读取电池 sysfs、结合充电会话估算健康度与容量，并把结果写入管理器的模块描述；模块自带一个三页 WebUI（概览 / 历史 / 设置），数据保存在本机 SQLite。

**全项目有且仅有两种叫法**：`CRecordEvolution`（模块 id、安装路径与数据目录、资产名前缀、代码标识符）与 `ChargingRecord Evolution`（展示名：`module.prop`/`module.yaml` 的 `name`、README 标题、WebUI 标题与关于卡、发布标题）。文档、UI、工作流新增文案只能用这两种，不要写 `ChargingRecord`、`Charging Record`、`ChargingRecord Evolution ML` 之类的变体；ML 变体与 stable 同名，只靠 `version` 后缀 `-ML`（模块 Version 字段）和资产名区分，**不改 `name`**，模块描述保持与 stable 一致的纯状态卡。不属于展示名的历史遗留保持原样：旧模块 id `Charging_Record`（README 升级提示、`describe_test.go` 夹具）与 `go.mod` 的模块路径 `charging_record`。

- 业务逻辑全部在 `cmd/batteryd/`；`service.sh`、`action.sh`、`customize.sh` 只是 POSIX sh 引导/交互壳，由 root 管理器执行。
- WebUI 源码在 `webui/`（Preact + Vite，ES module/JSX），构建到 `webroot/`（生成目录，不提交）。
- 支持 KernelSU / APatch / MMRL 的 WebUI；Magisk 通过标准 META-INF 刷入桩安装；只发布 arm64 静态二进制。
- 纯本机运行：无网络请求、上传或遥测。

### 两种构建变体

| 变体 | 构建差异 | 产物 |
| :-- | :-- | :-- |
| stable | 默认构建（`channel=stable`） | `CRecordEvolution-{versionCode}-{version}.zip` |
| ML 实验版 | 注入 `-X main.channel=ml`；CI 把 `version` 改成 `{version}-ML`、删 `updateJson`，`name`/`versionCode` 不变 | `CRecordEvolution_ML-{versionCode}-{version}.zip` |

两个变体 id、名称相同，互斥安装；`updateJson` 只指向 stable，ML 只能手动刷入。

## 构建与测试

### 本机命令

```sh
go test ./... -count=1
go vet ./...
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o bin/batteryd ./cmd/batteryd

pnpm install --frozen-lockfile
pnpm run verify   # build → 离线扫描 → 50KB gzip 上限
pnpm run smoke    # 无头 Edge/Chrome + mock 宿主，三页/分页/趋势/gate 截图
pnpm run shots    # 重新生成 README 的 8 张截图（390×844 深色；会覆盖 .github/images/）
pnpm run format   # Prettier 写回 webui/、scripts/、vite.config.js；format:check 只检查不写
```

- Go 版本以 `go.mod` 为准；SQLite 用纯 Go 的 `modernc.org/sqlite`（无 CGO，Windows 可直接跑测试）；Node/pnpm 版本由 `package.json` 固定。
- `bin/batteryd`、`webroot/`、`node_modules/`、`artifacts/` 都不提交。Vite root 在 `webui/`，入口 `webui/index.html`，产物固定到 `webroot/`；`pnpm run dev` 起本地开发服务器。
- **产物两条硬约束**：完全离线（CI 扫描 `fetch`/`XMLHttpRequest`/远程 URL/远程 import）与 gzip ≤50KB（`scripts/check-size.mjs`）；改依赖或大段代码后先跑 `pnpm run verify`。`build.modulePreload.polyfill` 必须保持关闭（它会注入 `fetch`）。

### WebUI 冒烟检查

改 `webui/` 后先 `pnpm run build`，再 `pnpm run smoke`（起静态服务、注入 mock `ksu.exec`、截图写到 `artifacts/smoke/`），或 `pnpm run dev` 手测。至少检查：三页切换且无 console 报错、`assets/*` 与 `boot.js` 无 404；设置页主题/纯黑/取色/语言/底栏（悬浮与经典两种，经典需断言 M3 80dp/24dp/12sp/64×32 指示器）；历史页 7/30/90 天与筛选、滚动加载、详情与趋势小图展开；无 `ksu` 时显示 gate 页而非白屏。

### 不能本地验证与测试要求

无 Android 真机：root 管理器执行脚本、KSU/APatch/MMRL WebView 的 `exec`/edge-to-edge/CSP、Magisk 安装升级流程只能通读推演；涉及这些路径必须声明「未在真机验证」并给最小验证步骤。

- 改 Go 逻辑必须补/改测试，`go test ./...` 全绿、`go vet ./...` 通过。
- 改 shell 至少 `bash -n customize.sh action.sh service.sh`；不要用 bash 语义「修正」POSIX sh，不在 Windows 跑安装流程。安装器**不选语言**（跟随系统/旧偏好）；**安装确认按风险分级**：普通升级/同版本重刷/首次安装直接进行，只有降级或 stable↔ML 跨通道才要求音量键确认；读不到 `getevent`/`timeout` 时自动安装并打印警告、不能误取消。
- 改 shell/元数据（三个 `.sh`、`module.prop`、`module.yaml`、`module.json`、`update.json`、`package.json`）会触发 CI 的 `meta` 工作流：bash 语法、CRLF、module.prop 必填键与 id/version/versionCode 格式、`package.json` 与 `module.prop` 版本一致、JSON/YAML 可解析。本地可照 `meta.yml` 的步骤自查；**改版本号必须同时改 `package.json`**，否则 CI 直接失败。
- 改 `module.yaml`/`module.json` 后用 YAML/JSON 解析器校验；改 README 任一侧同步另一侧并检查标题锚点。
- 本地打包确认 `.sh`、`module.prop`、`META-INF` 文本是 LF（CRLF 会让 mksh 报 `: not found`/`syntax error`）。

## 代码风格

### Go

- 使用 `gofmt`；新逻辑写成可测试纯函数，I/O 与状态机分层。
- 错误向上返回并保留上下文；影响结算/存储的错误不得静默吞掉。
- 面向用户字符串走 `uiText`/`uiKV`；内部日志、events 表、stderr 保持中文。
- 新增数据字段同步 `cmd/batteryd/jsonout.go` 与 WebUI，保持「缺失即省略、按可用字段降级」。

### 计量单位与命名约定

- **后缀即量纲**：`*UV`=µV、`*UA`=µA（带符号）、`*UAh`=µAh、`*UAs`=µA·s（÷3600 得 µAh）；新增字段必须带可判量纲的后缀。换算只在展示层（`mV`/`mAh`/`mΩ`），采样、积分、kv、`Snapshot` 与 JSON 出口都传原值。
- DB 列名是历史遗留、**不代表单位**：`sessions.ua`=会话累计电量 µA·s、`sessions.cc`=会话期采信的电量计差分 µAh（0=未采信）、`estimates.mah`=µAh、`samples.cc`=charge_counter 读数 µAh（0=未采到）、`discharge.uah`=µAh。以 Go 字段名与 `schemaDDL` 行尾注释为准。
- `counter` 指电量计 `charge_counter`；`CC`/`CCSeg`/`CCCT` 专指恒流充电段，两套词汇不得混用。
- kv 键名与 JSON tag 是设备端/前端契约：改名只改 Go 常量名，字符串值原样保留（`ema_ua`、`sess_cc_acc`、`sess_cc_last`、`dis_last_cc`、`cur_scale`；tag `uah_mah`、`peak_h_rel`、`current_scale`）。
- sysfs 节点名沿用内核原名（`charge_counter`、`current_now`、`charge_full`、`charge_full_design`）。

### POSIX sh

- shebang 固定 `#!/system/bin/sh`；不用数组、`[[`、`local` 等 bash 特性；变量加引号，路径用 `$MODPATH`/`$MODDIR`。
- 输出走 `ui_print`（缺失回退 `echo`），取消用 `abort`（缺失才 `exit 1`）；安装器总输出约 8~14 行。
- 安装器不再选择语言（2026-11 按 issue #9 定稿）：语言跟随系统/旧 `data/lang`（探测不到回退英文），安装器不写 `data/lang`。
- **安装确认按风险分级**（2026-11 issue #9 讨论后定稿）：首次安装、普通升级（versionCode 变大）、同版本同通道重刷都直接安装，不读音量键（管理器自己已有安装确认，KernelSU/APatch 有模块信息弹窗；Magisk App/recovery 无，但普通安装仍不额外打断）。只有检测到**降级**（目标 versionCode 更小）或 **stable ↔ ML 跨通道**时，才打印「已安装 vX(code) 通道 → 本次 vY(code) 通道」并要求音量键确认（音量+ 继续 / 音量- 取消，30 秒超时取消）。
- 通道判定：`module.prop` 有 `updateJson=` 行 = stable，没有 = ML（CI 打包 ML 时删掉该行）；`versionCode` 非数字/读不到时不比较。
- 应用内安装与恢复模式用同一套按键，**不按 BOOTMODE/管理器类型跳过**（2026-10 实机确认 LSPosed/Zygisk 系在 App 内也能读音量键）。只有读不到 `getevent`/`timeout` 时才自动安装，绝不能误取消；风险场景同样打印警告后继续。
- `read_key` 只采纳 `DOWN`，忽略 `UP`/`SYN`（否则一次按键的抬起会被当成两次）。回归：风险确认时按一次取消。
- `SKIPUNZIP=0` 时已解压到 `$MODPATH`，不要重复 `unzip`，仅缺 `module.prop` 时兜底一次。
- 只发 arm64：用 `$ARCH` 门控（缺失跳过），非 arm64 直接失败。
- 交互文案走 `T`/`L`，语言探测顺序与 `i18n.go` 一致（升级沿用 `data/lang`，否则系统语言，探测不到回退英文）；不把逻辑改动混进文案替换。
- 只给 `bin/batteryd` 设权限，**不要给 webroot 加 chmod/set_perm**。

### WebUI

- Preact 10.29.8 + hooks，源码在 `webui/src/`；**完全离线**：无 CDN/远程脚本字体/运行时网络请求；新增运行时依赖进 bundle 并更新 `THIRD_PARTY_NOTICES.txt`。
- 统一 JSX；不用 `h()`/`E()`，新代码拆小函数/组件，别把 `Overview`/`History` 重新写胖。**不引入 htm**（Apache-2.0 与 GPL-2.0-only 不兼容）。
- 静态图标放 `webui/src/assets/icons/*.svg`，由 `app.css` 的 `mask-image` + `currentColor` 着色；组件里不内联 `<svg>`。图表 SVG 由 `charts.js` 运行时生成。
- `webui/index.html` 只做入口壳；`public/boot.js` 在 `<head>` 同步加载；`/internal/insets.css` 用占位符交给 `vite.config.js`。
- 源码按职责分：`i18n`、`core`、`charts`、`ui/`（context/components/overview/overviewModel/settings）、`history/`（History/HistoryControls/HistorySummary/TimelineList/timeline/trends/useHistoryData）、`assets/icons/`、`useDocPolling`/`useUiPrefs`、`main.jsx`；模块间 import/export，不退回全局变量。
- 可测纯逻辑放同级 `*Model.js`（如 `buildOverviewModel(doc, lang, t)`），取数与副作用放 `use*` hook，组件只编排渲染；`App` 只组合 hook。
- `webroot/` 生成目录：不手改不提交；样式改 `webui/src/app.css`，逻辑改 `webui/src/`。
- 用户数据只用 `textContent`/Preact children；唯一 `dangerouslySetInnerHTML` 是 `charts.js` 内部生成的数据驱动 SVG。
- 可交互元素要有语义：分段/档位按钮带 `aria-pressed`、可展开的格子带 `role="button"` + `tabIndex` + `aria-expanded` 并支持 Enter/Space；可点区域不小于 40px（`.chip`/`.seg-btn` 已按此设高）；对比度不足的 `--faint` 已提到 ≥4.5:1，新增弱化色不得低于它。
- 图表 SVG 统一 `aria-hidden="true"`（信息由卡片标题/最新值/点按读数/caption 承载，读屏无法操作数据点），不要给无名称的 `role="img"`。
- 顶栏三页统一高度（`.topbar-inner` 的 `min-height`）；全局关闭 `-webkit-tap-highlight-color`、按钮 outline 与 user-select。

### 双语文案与格式

- Go 用 `uiText`/`uiKV`；WebUI 用 `LANGS` + `LangCtx` 的 `t()`/`plural()`（两份字典同时补 key，组件禁止硬编码中文）；shell 用 `L()`/`T`。内部日志、events、stderr 保持中文。
- 数字与复合单位留空格（`%d mAh`、`%d ℃`）；中文百分号/量词紧跟数字（`健康 92%`、`循环 210次`）。英文描述用 ` | `、中文用「｜」；日期按语言分流（`fmtTs`/`fmtRange`）。
- 英文单复数必须走 `plural(lang, n, oneKey, manyKey)`（如 `hist_records_count_one` + `hist_records_count`），不要手写 `{n} records` 让 1 条显示成 "1 records"。
- 模块描述是两行状态卡：`💚 内核 : 92% - 6700 mAh • ♻️ 循环 : 301次` / `📏 实测 : 90% - 6300 mAh • 🌡️ 温度 : 41.1℃`（英文 `Kernel`/`Cycles`/`Actual`/`Temp`；`Kernel` 与 `Actual` 同为 6 字符、字宽几乎相等，所以第一列数值不靠字体相关的补位也能对齐；其余标签 `满充`/`设计`/`Full charge`/`Design`）。分段固定为「图标 标签 : 数值」，格间中文/英文都用 ` • `，数值内 `% - mAh`。**对齐靠 `descRuneEm`（Roboto Regular 实测字宽，em 千分位）＋ Compose 字距 `descLetterEm`（KSU/Magisk bodyMedium 0.25sp/14sp、APatch bodySmall 0.4sp/12sp，每字符都加）**：同列标签补空格让数值同列、第一格补到等宽让 ` • ` 同列、行尾补到等宽；中文标签等宽天然对齐，英文 `Kernel`/`Measured`、`Cycles`/`Temp` 靠字宽差补位（残差 <1 空格 ≈1.5dp，OEM 字体可能略差）。KernelSU/APatch 写**转义 `\n`**（java-properties 会还原成真实 LF，Android 的 `PrecomputedText` 只按 `\n` 切段，U+2028 不换行）；Magisk 不还原转义，退化成单行「行1 · 行2」且不补位。管理器按 `/data/adb/ksu`、`/data/adb/apatch`、`/data/adb/magisk` 探测（测试用 `managerKindForDesc` 覆盖）。**宽度自适应**：`descAvailableEm()` 读 `wm size`（优先 Override size）、`wm density`（优先 Override density）与 `settings get system font_scale`，按 `(dp−64)/font_scale/14sp` 折成可用 em（进程内缓存一次；读不到按 360dp）。标签逐级裁剪：全标签 → 紧凑（去掉循环/温度标签，只留图标）→ 仅图标。实测阈值（em 千分位，中文/英文）：全标签 19569/21656、紧凑 16669/17696、仅图标 13877/13956；即 393dp 屏两者全标签，360dp 屏中文全标签、英文紧凑，360dp+1.15× 字体中文紧凑、英文仅图标。描述不再带 ML 前缀（通道见 Version 字段的 `-ML` 后缀），宽度只按状态卡计算。**`module.prop` 不得写真实 CR/LF**（`WriteModuleProp` 负责清洗）。

### 文档

- README.md（英文，GitHub 默认首页）与 README_ZH.md（中文）章节/锚点一一对应，语言导航行在徽章区之后；英文首页保留明确的中文入口；公开文档只写本项目行为，不写来源归属。
- 设计笔记、过程规格、调研报告放本地工作区不提交；`docs/` 只放按命名规范格式化的论文 PDF。

### 提交信息（Git）

- 格式：`type(scope): 中文单行主题`，冒号后一个空格。主题说明「改了什么/修了什么」，约 ≤40 汉字，不以句号结尾，**不写 issue 编号、不加英文括注**（issue 归属走正文/PR）。type 沿用历史：`feat`/`fix`/`docs`/`chore`/`add`/`refactor`/`ci`/`perf`；`add` 是仓库既有用法（新增文件/数据/资产），不必为「标准」改成 feat。
- scope 用模块级小写词，历史常用 `battery`（daemon 业务）、`calib`（电流/容量/电量计校准口径）、`webui`、`ui`、`installer`、`release`、`stats`、`git`、`tooling`、`ci`；跨多处时选改动主体，不要凭空造新 scope。
- 正文默认要有：用 `-` 列表写 2~6 条，覆盖「根因（现象→机理）、改动要点、测试/回归」，写清楚为什么，而不是罗列文件；发布、元数据、纯文档一句话能说清的可省略正文。
- 提交前先 `git log --format='%h %s' -10`（需要时加 `%b` 看正文）对齐既有风格；提交/推送后确认 CI（Go / WebUI）通过。
- 改写已推送提交（amend/rebase/force-push）必须先得到用户明确指示，并用 `--force-with-lease`。

## 仓库结构

| 路径 | 用途 |
| :-- | :-- |
| `cmd/batteryd/` | Go daemon：采样、结算、容量估计、SQLite、description、各子命令 |
| `service.sh` / `action.sh` / `customize.sh` | 启动壳 / Action 跑一次 `once` / 风险分级确认安装器 |
| `webui/` → `webroot/` | WebUI 源码（Vite）→ 构建产物（进 zip，不提交） |
| `scripts/` | 离线扫描、gzip 体积检查、无头冒烟（`smoke.mjs`）、README 截图（`screenshots.mjs`） |
| `module.prop` / `module.yaml` / `module.json` / `update.json` | 模块元数据与 KernelSU 展示/更新（`assetPattern` 只匹配 stable） |
| `META-INF/...` / `THIRD_PARTY_NOTICES.txt` | Magisk 刷入桩 / 随包第三方许可（Preact MIT） |
| `.github/workflows/` | release（双变体发布）/ webui（守卫+冒烟）/ go（gofmt+vet+test）/ meta（安装器 shell 与模块元数据守卫） |
| `.release-baseline` | 更新日志基线（上一版发布内容 commit），CI 回写，勿手改 |
| `README.md` / `README_ZH.md` / `docs/` | 中英文档必须同步 / 论文 PDF 库 |

运行时数据只在 `$MODDIR/data/` 内；历史查询按 90 天窗口取数，趋势点 ≤120 个/类，事件正文游标分页。

## WebUI 与宿主集成

### 取数协议

- 通过管理器 cbName 协议 `exec(cmd, '{}', 回调函数名)` 调用固定命令：`/data/adb/modules/CRecordEvolution/bin/batteryd json|once|history|diag`。
- `json` 顶层除概览/历史数据外还有两个实时块：`live`（status/cap/current_ua/voltage_uv/power_mw，`charging` 为进行中会话的 start_ts/start_cap/cap/acc_uah/secs/avg_current_ua/eta_min）与 `daemon`（心跳 version/ts/age_sec/stale）。两者字段缺失即省略：节点全读不到没有 `live`，`data/daemon.json` 缺失/损坏没有 `daemon`；旧 daemon 兼容靠这一点。`live.current_ua` 必须走 `normalizeCurrentUA`（已学刻度优先），**不得**下发原始读数；**符号约定固定为充电正、放电负**（`liveDirection` 按 status + 进行中会话判向，尾段 status=Full 仍按充电），不要跟随 `current_now` 原始符号——部分内核充电报负、放电报正（真机实测），透传会出现「充电中 -8.8A」；`power_mw = current_ua × voltage_uv ÷ 1e9`（双电芯总电压×总电流即总功率，不除电芯数，符号随电流）。ETA 由后端 `chargingEta` 纯函数算（参考满容量取 `charge_full`→`design`，速率优先本次会话平均电流，瞬时电流只在 status=Charging 时兜底，夹到 [1,1440] 分钟），只在充电方向（`liveDirection>0`）给出，放电中不给，前端只显示不推算。
- `diag` 只输出短文本（≤1800 字符）：机型/系统/root 管理器/电池节点/校准与最近会话摘要 + 最近一次校准或口径裁决 detail（≤160 字、带 `(Nd)` 时效）+ 最后采样时间；日志按关键词优先、启动行去重。**不得含序列号/IMEI/MAC/账号**；`getprop` 必须走 `diagAllowedProps` 白名单（manufacturer/model/device/release/sdk/abi），白名单外直接返回空。设置页「复制诊断」执行它写剪贴板（失败回退只读文本框）。
- 历史页用 `history --days 7|30|90 --limit 20 --type all|charge|discharge [--before TS]`：服务端合并充电/放电事件、按时间倒序游标分页，返回摘要与趋势点；`json` 保持原样供概览/设置。
- 15s 超时；`ksu` → `APatch`/`apatch` 级联回退；无 `ksu` 显示 gate 页。命令必须是常量或固定模板拼接；`history` 参数只接受数字与固定枚举，不得拼用户输入或远端数据。

### Edge-to-edge / 系统栏

- `<head>` 用根相对路径请求 `/internal/insets.css`；KSU/APatch WebView 会拦截并铺满系统栏、下发 `--window-inset-*`（新版 KSU 另有 `--safe-area-inset-*`）。
- `<head>` 同时用根相对路径请求 `/internal/colors.css`（KernelSU / APatch / MMRL 等宿主注入 M3 调色板）：没有提供时样式为空/404，靠 `--primary`/`--background` 探测降级，设置里不出现取色档；**不能改成远程 URL**（离线约束）。
- 宿主源码依据（均为 2026-11 核对，变量名统一 camelCase，来自 Compose `ColorScheme` 成员名，不需要按管理器分支）：
  - KernelSU（tiann/KernelSU `730f644`）：`manager/app/.../ui/webui/SuFilePathHandler.java` 处理 `internal/colors.css`——**v3.0.0（2025-11-29）起**提供；仅当 `color_mode` 为 3~6（MONET_SYSTEM/LIGHT/DARK、DARK_AMOLED）或 `ui_mode=material` 时返回 `MonetColorsProvider` 的 CSS（v3.2.0 前只认 3~5），否则返回 **200 空 body**；`WebViewAssetLoader` 域名 `mui.kernelsu.org`、路径处理器挂在 `/`，根相对请求全被拦截，不会走网络（虽然 App 声明了 INTERNET 权限）；更老版本是 404。
  - KernelSU-Next（`035dbae`）：同文件，仅 Android 12+ 提供，`MonetColorsProvider.getColorsCss(context)` 始终返回系统动态色。
  - SukiSU-Ultra（`fae5788`）：同文件，门控与 KernelSU 相同（Monet 配色或 Material 界面）。
  - APatch（bmax121/APatch `e8e4651`）：`app/.../ui/webui/SuFilePathHandler.java` + `MonetColorsProvider.kt`，**build 11219（2026-08-02）起**提供，无开关门槛；`WebUIActivity` 组合 `APatchTheme` 后始终有 `MaterialTheme.colorScheme`（Android 12+ 且开启动态色才是壁纸色，否则为内置主题色）；组合完成前为空，前端用「重试 + 重挂 stylesheet（`?retry=N`，宿主按路径拦截）」兜底晚生成；更老版本 404。
  - MMRL / WebUI-X（MMRLApp/WebUI-X-Portable `3d87c20`）：`InternalPathHandler.kt` 的 `route("/internal/colors.css")` + `WebColors.kt`，**v212（2025-08-08）起**始终由 App 主题生成；当前 WebUI 引擎 `dev.mmrlx:webui.core` 闭源，拦截按旧版开源实现与 API 推断，未真机抓包。
  - Magisk（topjohnwu/Magisk `33bfff7`）：App **没有模块 WebUI 宿主**（无 `webroot`/`internal/insets.css`/`internal/colors.css` 处理），Magisk 只负责刷入；`webroot/` 只能被上述管理器打开。
- 宿主的 `colors.css` 变量集合覆盖 `--primary/--onPrimary/--surfaceContainer*/--outlineVariant/--background/--onBackground/--error` 等（KSU/APatch 硬编码 45 个；MMRL 反射 `ColorScheme`，可能多 `--primaryFixed*`；`--shadow` 实际不存在，文档有笔误）；缺失时前端退回内置三套主题，设置里不显示取色档。
- **明暗方案必须从宿主 `--background` 的相对亮度推导**，不能用 `prefers-color-scheme`：KSU/APatch 的 WebView 没有随管理器 `color_mode` 设置 dark 标志，`matchMedia` 不可信。`data-scheme`、`color-scheme`、`setLightStatusBars`、AMOLED 是否生效都跟随推导结果。
- head 里尽早 `ksu.enableEdgeToEdge(true)`，旧接口 `enableInsets(true)` 兜底；挂载时、350ms、`load` 各再调一次。
- 回退链：`--safe-area-inset-*` → `--window-inset-*` → `env(safe-area-inset-*)` → 0。**不删 insets.css 请求，不写死 inset**。
- 宿主没 edge-to-edge 不是 bug（KSU ≤2.1.2 / APatch ≤11142 本就内缩），页面不要再加 padding。
- **不要画 `#sysbar`/`#navbar` 对比带**（主题判断不一致会出黑白条）；inset 区交给页面背景；MMRL 的 legacy `setLightStatusBars()` 用 `syncStatusBarStyle()` 同步。

### 三页与设置项

- 概览：健康度主卡（容量行下方温度行 `temp_c`，缺失整行省略）、充电中实时卡（`live`）、容量/循环/估算/内阻/趋势/可信度、估算趋势图。充电实时卡只在 `live.charging` 存在或 `status=Charging` 时渲染，显示实时电流/功率/电压、本次会话进度与预计充满，字段缺失逐段省略；电流/功率符号按上面的「充电正、放电负」约定显示（放电中不显示预计充满）；会话进度与预计充满**合并为一行**（`.charge-meta`，`nowrap`，390dp 不溢出，smoke 断言 `scrollWidth` 与单行高度）。充电期轮询 15s、其余 60s（`POLL_CHARGE`/`POLL_BASE`），`chargeActive()` 是两处共用的唯一判据。`contentFP` 按 0.1℃ 纳入 `temp_c`，剔除 `calib.raw_current`/`current_ua`（每次 json 都变、不展示），并把 `live` 按显示粒度取整（电流 10mA、功率 0.1W、电压 0.01V、会话 1mAh/60s），否则实时值会被去重冻结；`recent` 与 `trend_mah_per_week` 共用最近 200 点窗口；趋势显著时 JSON 给 `trend_from_mah`/`trend_to_mah`（窗口首/末点容量），前端直接连两端点画虚线，不要下发斜率让前端外推。
- 历史：摘要统计；记录视图按日期分组时间轴（范围+类型合并成一行 chip：7/30/90 天、全部/充电/放电，分页「加载更早」）；趋势视图为容量与充电会话大图 + 放电/静置/CCCT/ICA 小多图（小图点按展开）；大图与展开图都支持点按数据点读数并高亮（`charts.js` 接受可选选中索引，只拼内部数值）；详情渐进展开。**切到别的页再回来不重载**：分页、展开详情、滚动位置都保留（只有首次激活、切天数/筛选/视图、手动刷新才 `load(true)`）；「加载更早」失败时在列表底部渲染错误行 + 重试（`err.reset` 决定重试整页还是同一页游标），首屏失败仍用空态错误样式。
- 设置：外观（主题/纯黑/取色/语言/底栏）、手动刷新、数据信息（含采样节点与电流刻度诊断，来自 JSON 顶层 `calib`，末尾「复制诊断」）、关于（品牌卡 + 版本/通道/许可/第三方）。`json` 的 `sessions` 只有最近 10 条，「会话数」必须用顶层 `sessions_n`；会话条目 `start_cap`/`end_cap`，趋势卡与时间轴百分比是 **SOC 百分点变化**（70%→97% 是 27 个点），不是容量增长。`daemon.stale` 为 true 时数据区追加一行「已升级，重启后生效」（`data-field="daemon"`），不 stale 时该行不出现。
- 底栏悬浮胶囊 / 贴底（`cre_barmode`，默认悬浮）；**`classic` 按 Material 3 Navigation bar 规范**：容器 `--nav-bg`（内置=卡片色，取色=`surfaceContainer`）、高 80dp、图标 24dp、label 12sp/500、激活指示器 64×32dp 全圆角 `--nav-ind-bg`（取色=`secondaryContainer`）、激活图标 `onSecondaryContainer`、激活 label `onSurface`、未激活 `onSurfaceVariant`；悬浮态沿用原胶囊但选中底/字改用同一组 token。`cre_theme` 为 `auto|light|dark|my`（`my` 仅宿主提供 M3 调色板时可选中，明暗从调色板 `--background` 亮度推导，`data-scheme` 承载方案）；`cre_amoled` 为独立纯黑开关（只在解析为深色时把底色压到 `#000`，浅色不受影响；`data-black` 属性承载）；`cre_lang`。切页保存/恢复滚动；设置页隐藏顶栏刷新，用页内「立即刷新」。

### 语言文件

- 优先级：`data/lang`（WebUI 写入，旧安装器遗留）> localStorage `cre_lang` > `navigator.languages`；探测不到回退英文（`i18n.go`/安装器/`boot.js` 三处一致）。
- 切换语言时原子写 `data/lang`（临时文件 + `mv`），再跑一次 `batteryd once`；连续切换要排队，避免旧写入覆盖新选择。

## 领域不变量与已知坑

### 采样与会话结算

- 采样间隔：充电期 15s，其余 60s；满电后不回封账，CV 尾段由 `tickTailCharge` 延续累计，电流停歇去抖后结算。
- 会话电量双口径：电量计差分（首选）与电流积分。两者差 3 倍以上时用容量参考（设计容量，缺失用 `charge_full`）做 0.5~1.5 倍窗口裁决：只有「电量计在窗内、积分在窗外」才保留电量计，否则回退积分；3 倍失配裁决时写 `counter_scale` 事件。
- `chargeSuppressed`：满电后高负载时内核仍报 Charging 且电流为正，但端电压较充电峰值回落 >`30mV × 电芯数`，视为直供假电流，不计入会话电量与吞吐（只作用于积分口径）。

### 容量估计、校准与趋势

- 构建注入 `main.channel` 选择 stable/ml Estimator。
- 中段校准 `midwin`：充电样本按 1% 显示百分点分桶、按穿越次数归一，聚合 `[30%, 90%)`；每日至多一次按 1/10 权重并入 EMA。区间电量优先取 `samples.cc`（实为 counter 读数）差分；有 counter 的样本里缺 counter 的旧区间直接跳过，整批无 counter 才回退积分。隐含值必须过 `capacityInWindow`，最近采信值存 kv `mid_implied_ua`。
- 放电通道 `discharge`：读 `charge_counter` 差分累计，转充电即结算；样本按 5 分钟降采样。门控：掉幅 ≥30 点、起始 ≤95%、过 `capacityInWindow`；按 1/10 权重并入 EMA，采信值存 `dis_implied_ua`。不经 `current_now`，是四路中最稳的一路。
- 质量哨兵 `quality`：ema/mid/dis/full 四路互检，互差 >40% 判不一致，结论翻转才写 `quality_warn`；可用路数 <2 时省略 `quality`。mid/dis 带时间戳（`mid_implied_ts`/`dis_implied_ts`），超 90 天不参与互检；缺 ts 的旧库首次读取按「当天」回填并宽限一次。时效判定在结算侧 `checkQuality`（写 ts、落 `quality_stale`）与 `stats()` JSON 出口（只读过滤）共用同一纯函数，改一处必须同步另一处。只记事件，不改 JSON 契约。
- 趋势 `trend`：三态 `insufficient|stable|significant`，显著性用 Mann-Kendall 点对符号检验（双侧 5%）；R² 门控已废弃（单次估算噪声远大于真实周衰减）。

### sysfs 与单位

- `NormCurrentUAWithFull` 按 `charge_full` 的**量纲**判电流单位：≥100000 只可能是 µAh，同驱动 `current_now` 已是 µA；mAh 则 ×1000；无锚点退化幅值启发式；最后 25A 物理上界兜底。**不要拿 `charge_full` 当幅值阈值**（曾把 <0.6A 的真实 µA 读数整批 ×1000）。
- `current_now` 量纲随内核不同（欧加系报 mA，同节点 `charge_counter`/`charge_full` 报 µAh）；生产路径一律走 `Pipeline.currentUA`（`normalizeCurrentUA`：已学刻度优先，未校准退回锚点），**禁止把原始读数当 µA 直接积分**。
- `charge_counter` 量纲也可能不守 ABI：荣耀 Magic7（issue #7）上 `charge_counter` 报 mAh、`current_now` 报 mA，而 `charge_full`/`charge_full_design` 仍是 µAh。**所有生产读取必须走 `normCounterUAh(raw, charge_full, cap%)`**（双锚点：`charge_full` 是 µAh 量纲、读数 < `full/500`，且 raw×1000 落在 `cap% × charge_full` 的 0.5~1.5 倍内 ⇒ ×1000；显示电量 0/缺失时一律不缩放，避免把 µAh 机低电量边缘读数放大 1000 倍），充电会话、放电差分、`samples.cc`、`sessions.cc`、diag 展示都不例外；只有迁移判定可用 `counterUnitDecided` 区分「确定 µAh / 确定 mAh / 无法判定」。**不要用相对比值判 counter 单位**：电流自校准的比值由 counter/积分反推，两路同时 milli 化时比值相消（≈1.4），会把错误刻度固化成 ×1；必须先归一 counter，电流刻度才能学到 ×1000（回归见 `TestIssue7CounterMilliampDevice`）。
- 运行时自校准：充电结算时用「电量计差分 vs 会话期原始电流积分 `rawUAs`」反推 µA/原始单位，**不能用已缩放的 `accUAs` 反推**（否则 ×1000 与 ×1 来回震荡，回归见 `TestIssue6MilliampCurrentCalibration` 第二会话段）。比值须落在 10 的整数次幂 ±0.4 decade（≤2.5 倍偏差；<0.5 保证吸附区不重叠）且结算已采信电量计口径才学习；采信值写 kv `cur_scale`，事件 `cur_cal` 记落盘/变化、`cur_cal_fail` 记写库失败或比值不达标、`counter_scale` 记口径裁决。允许覆盖旧刻度（OTA 改单位后下个会话自动纠正）。
- `sessions.cc` 存结算时采信的电量计增量 µAh（被门控拒绝也照存；0=无或未采信）；历史页「估算」优先由它折算，缺才回退积分。`sessions.ua` 仍是积分口径，两者不混用。
- `NormTempC` 截断 `[-40, 80]°C`；`readDTBatteryCapacity` 在 `charge_full_design` 缺失时读设备树 `vivo,bat-capacity-mah`（绝对路径优先）；`current_now` 带符号读取（放电为负），其余节点严格非负。

### 时间与存储

- 设备 `time.Now()` 可能是 UTC：面向用户的时间戳一律 `localNow()`（getprop `persist.sys.timezone` → 文件兜底 → UTC，内嵌 `time/tzdata`）；Pipeline 注入时钟同样用 `localNow`。
- SQLite 十表：kv/sessions/estimates/resistance/rest_points/events/samples/ccct/ica_peaks/discharge；90 天清理。WAL + `wal_autocheckpoint=256` + 每日 `wal_checkpoint(TRUNCATE)`。
- daemon 每 tick（循环前先写一次）原子写 `data/daemon.json`（临时文件 + rename，`{version,pid,ts}`）；`json`/`diag` 读它判断「已升级但旧 daemon 仍常驻」。心跳写失败静默，绝不阻塞采样；`json` 的 `daemon` 与 diag 的 `daemon=` 行都按文件缺失整体省略（旧版兼容）。
- 旧库升级做幂等修复：`repairSampleUnits` 按 25A 上界把误判样本 ÷1000（kv `samples_unit_repair`）；`RepairCounterUnits` 按「counter 读数 + charge_full + 显示电量」判定 mAh 口径机型，把历史 `samples.cc`/`sessions.cc`/`discharge.uah`/`discharge.implied` 与进行中会话 kv 一次性 ×1000（kv `counter_unit_repair`）。迁移以 kv `counter_unit_epoch`（新代码首次启动时刻）为截止，**只处理该时刻之前写入的行**：后续启动复用同一 epoch 重试也不会碰到已归一的新行；读不到节点/量纲不确定时本次不判定、不落终态标记，下次启动重试。不迁移 estimates/ema/mid_implied（可能来自积分口径）；`samples`/`sessions` 补 `cc` 列（旧行默认 0）。
- `module.prop` 的实时 description 用临时文件 + rename 原子写回，不要改成 sed。

### 关键数值与口径（勿随意改）

- 双电芯判定阈值是 **5V**：4.5V 会误判高压单电芯（截止 4.45~4.53V），导致电压窗口翻倍、CCCT/ICA 全哑。
- `initCCCTVoltage`/`initICAVoltage` 必须无条件赋值以支持复位，否则测试间全局状态污染。
- **不要对 `charge_full` 乘电芯数**（不同内核口径不同，已是整包会翻倍）。
- **不要拿 `current_now` 积分当绝对容量口径**（实机同 SOC 区间只有放电差分的 82~95%）；结算与中段校准首选 `charge_counter` 差分，积分只在 counter 不可用/刻度异常时兜底。

### 已知局限（有意取舍，勿当 bug）

- 电流单位启发式在涓流 <10mA 可能误判；RLS 无遗忘因子、P 矩阵长期膨胀是潜伏项；`FindNode` 全树兜底只在缺节点时触发。
- `charge_counter` 的 mAh 判定需要显示电量做交叉校验：显示电量为 0/不可读时读时归一不缩放、历史迁移本次不判定（下次启动重试）；`charge_full` 缺失或本身以 mAh 计时不归一（保持既有设备口径）。升级后旧 daemon 仍在内存里跑、继续按旧口径写库：心跳只能**提示**（`daemon.stale` → 设置页「已升级，重启后生效」），不会自动重启/替换守护进程，**升级后需要重启**；重启前旧 daemon 写的行仍可能带旧量纲且不会被迁移（`counter_unit_epoch` 之后的行按已归一口径处理）。
- 充电实时卡的 ETA 用「本次会话平均电流」估算，CV 尾段电流自然衰减，估计值会偏乐观；旁路供电/极涓流或 `charge_full` 缺失时可能不显示，仅作参考，不参与任何结算。
- Material You 调色板由宿主在**页面加载/主题组合时**生成并随 CSS 一起下发：WebUI 打开期间切换系统明暗或更换管理器配色不会重新取色（取色模式下的 `data-scheme` 也按加载时调色板的背景亮度固定，配色自洽）；重开 WebUI 即刷新（重挂 stylesheet 加 query 可重取，KSU/APatch 忽略 query）。KernelSU/SukiSU 未开启 Monet（`color_mode` 0~2 且非 Material 界面）时 `colors.css` 为空 → 取色档不出现，属于预期。不要为此引入远程请求。
- 节点缺失时描述/JSON/once 按可用字段降级，不整体失败。
- 放电会话只在 `status=Discharging` 时累计；旁路供电机型（Full 但实际放电）会漏记。
- 放电隐含容量以「显示掉幅」为分母，顶部区间被压缩、绝对值偏高，只作观测不进估算通道。
- 电流刻度自校准只在充电结算发生：学到新刻度会按新刻度重算该会话积分电量/平均电流/倍率（`sessions.ua`、`avg_i`、`c_rate`），并按比例回填该区间的 `samples.ua`、`resistance.mo` 与 `ica_peak_base`（事件 `cur_backfill`/`cur_backfill_fail`/`cur_backfill_skip`）；仅当旧有效刻度可确定（已学 >0，或 `anchorScaleUA(charge_full)`=1/1000）才回填。回填不覆盖 `charged_ua_total` 与历史行；无 counter 的机型缺标尺无法自校准；已发布库的坏基准无法自动识别。

## 安全注意事项

- WebUI 的 `exec` 在 root 下执行，命令必须固定字符串/模板；**绝不**把用户输入、语言值、模块描述、DB 内容拼进 shell。
- 不引入远程脚本、CDN、遥测或上传；数据只留本机；运行时依赖由 lockfile 固定并随产物附许可证。
- 不给 `webroot` 加 chmod/set_perm；安装器只处理 `bin/batteryd`。
- 不提交 `bin/`、`dist/`、生成 zip、`data/`、`*.exe` 或密钥/签名文件；图表 SVG 只用内部生成数据。

## 发布流程

0. Releases 必须开启 **Enable release immutability**（否则 KernelSU 报 `release-not-immutable`，版本作废）。
1. 同步改 `module.prop` 的 `version` 与 `versionCode`，严格递增，不复用历史值。
2. 提交 main（只提交源码；`webroot/` 不入库）。
3. `git tag vX.Y.Z && git push origin main --tags`。CI：校验 tag↔version → go vet+test → Go 构建 → `pnpm install --frozen-lockfile` + `verify` → 打包 stable/ML → KernelSU 校验器 dry-run → draft Release 上传 → 发布（immutable）→ 回写 `update.json` 与 `.release-baseline`。
4. tag 必须与 `module.prop` 的 `version` 一致，否则 CI 直接失败。
5. 更新日志基线用 `.release-baseline`（上一版发布内容所在 commit），CI 发版后回写，勿手改；**不能**改用 `git describe --tags` 找上一版（tag 可能与 main 脱节，测试 tag 会干扰）。`release.yml` 生成 changelog 时会过滤 CI 自己发版后提交的 `add: update.json …` 行（否则会出现在 KernelSU 更新弹窗里）。
6. 发版 tag 必须落在 main 历史上；临时测试 tag 用完即删。
7. 资产名：`CRecordEvolution-{versionCode}-{version}.zip`、`CRecordEvolution_ML-{versionCode}-{version}.zip`、`changelog.txt`；`assetPattern` 只匹配 stable；两个 zip 都含 `THIRD_PARTY_NOTICES.txt`。

## 提交前检查清单

- [ ] 已读相关源码和 README/AGENTS 约定，没有覆盖用户未提交的改动。
- [ ] 提交信息符合「提交信息（Git）」：`type(scope): 中文单行主题`，正文写根因/改动/测试（已先看 `git log` 对齐风格）。
- [ ] Go 改动有测试；`go test ./...`、`go vet ./...` 通过。
- [ ] shell 改动通过 `bash -n`，并说明无法在真机验证。
- [ ] WebUI 改动通过 `pnpm run verify` 与 `pnpm run smoke`（或真机/模拟宿主），三页/设置/图表/分页都检查过。
- [ ] 没有把 `webroot/`、`node_modules/`、`artifacts/` 加入提交。
- [ ] 新增 UI 文案同时更新 zh/en；新增 JSON 字段两端同步。
- [ ] 需要发布的版本才改 `module.prop` 版本号；不提交 `bin/`、`dist/`、zip。
- [ ] 本地打包时 `.sh`、`module.prop`、`META-INF` 是 LF。
- [ ] 没有提交密钥、签名、数据文件或本地调试产物。
- [ ] 未经用户明确要求，不自行 `git commit`、打 tag 或触发发布。
