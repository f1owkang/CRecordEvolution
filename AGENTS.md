# AGENTS.md

本文件面向在本仓库工作的编码代理，说明项目目标、构建/测试方式、代码约定、领域不变量和发布流程。面向用户的介绍、安装步骤和功能说明在 [README.md](README.md) / [README_EN.md](README_EN.md)，不在这里重复。

## 项目概览

ChargingRecord Evolution 是一个 Android 模块（id=`CRecordEvolution`），用 Go 二进制 `batteryd` 读取电池 sysfs、结合充电会话估算健康度与容量，并把结果写入管理器的模块描述；模块自带一个三页 WebUI（概览 / 历史 / 设置），数据保存在本机 SQLite。

- 业务逻辑全部在 `cmd/batteryd/`。仓库里的 `service.sh`、`action.sh`、`customize.sh` 只是 POSIX sh 引导/交互壳，由 Android 上的 root 管理器执行。
- WebUI 源码在 `webui/`（Preact + Vite，ES module/JSX），构建到 `webroot/`；模块 zip 里带的是构建产物，`webroot/` 不提交。
- 支持 KernelSU / APatch / MMRL 的 WebUI；Magisk 通过标准 META-INF 刷入桩安装。
- 只发布 arm64 静态二进制，支持 Android 设备 root 管理器，不需要在设备外运行 daemon。
- 所有数据留在设备本地，没有网络请求、上传或遥测。

### 两种构建变体

| 变体 | 构建差异 | 产物 |
| :-- | :-- | :-- |
| stable | 默认构建（`channel=stable`） | `CRecordEvolution-{versionCode}-{version}.zip` |
| ML 实验版 | 构建时注入 `-X main.channel=ml`；CI 同时改名、description 加 `[ML实验版]`、删除 `updateJson` | `CRecordEvolution_ML-{versionCode}-{version}.zip` |

两个变体模块 id 相同、互斥安装；`updateJson` 只指向 stable，ML 只能手动刷入。

## 构建与测试

### 本机命令

```sh
go test ./... -count=1
go vet ./...
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o bin/batteryd ./cmd/batteryd

pnpm install --frozen-lockfile
pnpm run verify   # build → 离线扫描 → 50KB gzip 上限
pnpm run smoke    # 无头 Edge/Chrome + mock 宿主，三页/分页/趋势/gate 截图
pnpm run format   # Prettier 写回 webui/、scripts/、vite.config.js；format:check 只检查不写
```

- Go 版本以 `go.mod` 为准；SQLite 使用纯 Go 的 `modernc.org/sqlite`，不需要 CGO，Windows 本机可以直接跑测试。
- `bin/batteryd` 是构建产物，不得提交；`batteryd.exe` 等本地残留也不要提交。
- Node 版本以 `package.json` 的 `engines` 为准，pnpm 版本由 `packageManager` 固定。`webroot/` 是生成目录，不提交；不要直接改 `webroot/` 里的文件。
- WebUI 源码入口是 `webui/index.html`，Vite root 在 `webui/`。`pnpm run dev` 可起本地开发服务器；产物路径由 `vite.config.js` 固定为 `webroot/`。
- 构建产物必须满足两条硬约束：完全离线（CI 扫描 `webroot/` 里的 `fetch`/`XMLHttpRequest`/远程 URL/远程 import）和 gzip 上限 50KB（`scripts/check-size.mjs`）。改依赖或改大段代码后先跑 `pnpm run verify`。

### WebUI 冒烟检查

修改 `webui/` 后先 `pnpm run build`，再二选一：

- 直接运行 `pnpm run smoke`：脚本会起本地静态服务器、注入 mock `ksu.exec`，在无头 Edge/Chrome 里检查三页切换、历史分页/展开、趋势小图展开、主题/语言切换、gate 页，并把截图写到 `artifacts/smoke/`；
- 或 `pnpm run dev` 用桌面浏览器手动检查同样的路径。没有宿主时只能看到 gate 页，要看数据页需 mock `ksu` 或真机。

最低检查项：

- 三页可以切换，控制台没有 JS 报错；构建产物里的 `assets/*.js`、`assets/*.css`、`assets/*.svg`、`boot.js` 都能加载（无 404）；
- 设置页主题、语言、底栏样式切换正常；
- 历史页可以切换 7/30/90 天范围、筛选充电/放电、滚动加载更早记录，并展开记录详情；
- 历史页的“趋势”视图有小多图，点按可展开大图；
- 在没有 `ksu` 的普通浏览器里显示 gate 页而不是白屏。

### 不能在本地验证的内容

本仓库没有 Android 真机环境。以下内容只能通读代码推演，不能把本地 `bash`/PowerShell 行为当成设备行为：

- root 管理器如何执行 `customize.sh`、`service.sh`、`action.sh`；
- KernelSU/APatch/MMRL WebView 的 `exec` 回调、edge-to-edge、CSP 和系统栏行为；
- Magisk/KernelSU 的模块安装/升级/数据保留流程。

如果改动涉及这些路径，必须明确告诉用户“未在真机验证”，并给出最小验证步骤。

### 测试要求

- 改 Go 逻辑必须补/改测试，并保证 `go test ./...` 全绿、`go vet ./...` 通过。
- 改 shell 至少运行 `bash -n customize.sh action.sh service.sh`；不要用 bash 语义“修正”POSIX sh，也不要在 Windows 上执行安装流程。改安装器要推演「应用内安装与恢复模式同样可读音量键」的两步按键路径，以及读不到 `getevent`/`timeout` 时自动安装、不能误取消；本地无法真机验证时须说明。
- 改 `module.yaml` / `module.json` 后用 YAML/JSON 解析器校验。
- 改 README 任一侧必须同步另一侧，并检查标题锚点。
- 打包本地测试 zip 时确认所有 `.sh`、`module.prop`、`META-INF` 文本文件是 LF；CRLF 会让设备端 mksh 报 `: not found` / `syntax error`。

## 代码风格

### Go

- 使用 `gofmt`；新增逻辑优先写成可测试的纯函数，I/O 和状态机分层。
- 错误向上返回并保留上下文；不要静默吞掉影响结算/存储的错误。
- 面向用户的字符串必须走 `uiText` / `uiKV`，内部日志、events 表、stderr 保持中文。
- 新增数据字段时同步 `cmd/batteryd/jsonout.go` 与 WebUI，保持“缺失即省略、按可用字段降级”的纪律。

### 计量单位与命名约定

基准单位一律是微单位，换算只在展示层（`mV` / `mAh` / `mΩ`）做；采样、积分、kv、`Snapshot` 与 JSON 出口都传原值。

| 后缀 | 实际单位 | 典型符号 |
| :-- | :-- | :-- |
| `*UV` | µV（电压） | `peakUV`、`VwLo` / `VwHi`、`vStartUV`、`icaSearchLoUV` |
| `*UA` | µA（电流，带符号） | `iUA`、`MeanUA`、`maxCurrentUA`、`restQuietUA`、`samples.ua`、`avg_i`、`NormCurrentUAWithFull` |
| `*UAh` | µAh（容量） | `DesignUAh`、`fullUAh`、`EstUAh`、`CounterUAh`、`DischargeRow.UAh`、`kvKeyEmaUAh`、`chargeFullUAh` |
| `*UAs` | µA·s（电量积分，÷3600 得 µAh） | `Session.AccUAs`、`sessionState.accUAs`、`charged_ua_total`、`cycleEquiv(totalUAs, …)` |

- **后缀即量纲**：容量一律 `*UAh`，电流一律 `*UA`，累计电量 `*UAs`。不再存在"看语境才知道是 µA 还是 µAh"的 `*UA`；新增字段必须带能判量纲的后缀。
- DB 列名是历史遗留，**不代表单位**：`sessions.ua` 存会话累计电量 µA·s（不是电流），`sessions.cc` 存会话期采信的电量计差分 µAh（0 = 无电量计/未采信），`estimates.mah` 存 µAh（不是 mAh），`samples.cc` 存 `charge_counter` 读数 µAh（0 = 未采到），`discharge.uah` 存 µAh。以 Go 字段名与 `schemaDDL` 的行尾单位注释为准。
- `counter` 指电量计 `charge_counter`（`SampleRow.CounterUAh`、`sessionState.counterAccUAh`）；`CC` / `CCSeg` / `CCCT`（大写）专指恒流充电段。两套词汇不得混用。
- kv 键名与 JSON tag 是设备端/前端契约：改名只改 Go 常量名，**字符串值必须原样保留**（如 `"ema_ua"`、`"sess_cc_acc"`、`"sess_cc_last"`、`"dis_last_cc"`、`"cur_scale"`），JSON tag（如 `uah_mah`、`peak_h_rel`、`current_scale`）同理。
- sysfs 节点名（`charge_counter`、`current_now`、`charge_full`、`charge_full_design`）沿用内核原名，不要另起别名。

### POSIX sh

- shebang 固定 `#!/system/bin/sh`；不使用 bash 数组、`[[`、`local` 等非 POSIX 特性。
- 变量加引号，路径优先使用 `$MODPATH` / `$MODDIR`；不要写死 `/data/adb/modules/...` 以外的路径。
- 安装器控制台输出走 `ui_print`（管理器未提供时回退 `echo`），取消用 `abort`（无 `abort` 才 `exit 1`）；安装器总输出控制在约 8~14 行（标题/说明、设备、语言选择四行、确认提示两行、结果、一条提示），详细说明放 README/WebUI。
- 语言选择步固定输出中英双语、一行一个选项（标题、音量+ 行、音量- 行、超时说明）：此步语言偏好尚未确定，双语并列可同时覆盖两种语言的用户；长句塞进一行会在控制台折行，看起来挤在一起。选定语言后其余步骤用选定语言渲染。读键工具缺失走自动安装时也要说明语言跟随系统。
- 应用内安装与 Magisk 恢复模式都使用同一套两步按键（选语言 → 确认安装）；**不要按 BOOTMODE/管理器类型跳过按键**——LSPosed、Zygisk 系模块在 App 内安装也能读到音量键（2026-10 实机确认）。只有读不到 `getevent`/`timeout` 工具时才自动安装，绝不能因此误取消。
- `read_key` 只采纳 `DOWN`（按下）事件，忽略 `UP`/`SYN`；否则第一次按键的抬起会被第二步确认当成第二次按键（实机曾出现“按一次就安装”）。回归模拟：只按一次应取消，按两次才安装。
- `SKIPUNZIP=0` 时安装器已解压到 `MODPATH`，不要再重复 `unzip`；仅在缺 `module.prop` 的异常环境兜底一次。
- 只发布 arm64：安装器用 `$ARCH`（缺失时跳过）门控，非 arm64 直接失败。
- 安装器交互文案走 `T` / `L` 双语辅助函数，语言探测顺序与 `i18n.go` 保持一致；不要把逻辑改动混进文案替换。
- 安装器只给 `bin/batteryd` 设权限，**不要给 `webroot` 加 chmod/set_perm**。

### WebUI

- 使用 Preact 10.29.8 + hooks（npm 依赖，MIT），源码在 `webui/src/`；Vite + `@preact/preset-vite` 构建到 `webroot/`。**保持完全离线**：不加 CDN、远程脚本/字体或运行时网络请求；新增运行时依赖必须进 bundle 并更新根目录 `THIRD_PARTY_NOTICES.txt`。
- 组件统一用 JSX（`.jsx`）；不要再用 `h()` / `E()` 工厂，新代码优先拆小函数/组件，避免再长出几百行的 `Overview` / `History`。**不要引入 htm**（Apache-2.0 与本项目 GPL-2.0-only 不兼容）。
- 静态图标放 `webui/src/assets/icons/*.svg`，由 `app.css` 的 `mask-image` 引用并通过 `currentColor` 着色；不要在组件里内联 `<svg>`。图表 SVG 是数据驱动的，仍由 `charts.js` 在运行时生成，不能预存成静态文件。
- `webui/index.html` 只做入口壳；`webui/public/boot.js` 保持在 `<head>` 里同步加载（首屏主题/语言/底栏），`/internal/insets.css` 用占位符交给 `vite.config.js` 替换，不要改成 Vite 能解析的项目内资源。
- 源码按职责分：`i18n`（字典）、`core`（宿主协议、格式化、存储/主题）、`charts`（数据驱动的 SVG 纯函数）、`ui/`（context / components / overview / overviewModel / settings / Stat.jsx）、`history/`（History / HistoryControls / HistorySummary / TimelineList / timeline / trends / useHistoryData）、`assets/icons/`（静态图标）、`useDocPolling` / `useUiPrefs`（取数轮询与偏好落盘）、`main.jsx`（组合 hooks 与挂载）。模块之间用 import/export，不要再回退到全局变量。
- 可测的纯逻辑不要留在组件里：派生模型放同级 `*Model.js`（如 `ui/overviewModel.js` 的 `buildOverviewModel(doc, lang, t)`），取数与副作用放 `use*` hook，组件只做编排与渲染。`App` 只组合 hook，不在组件内直接写轮询/存储逻辑。
- 产物必须满足两条硬约束：完全离线（CI 扫描 `fetch` / `XMLHttpRequest` / 远程 URL / 远程 import）和 50KB gzip 上限；`build.modulePreload.polyfill` 必须保持关闭（Vite 的预加载 polyfill 会注入 `fetch`）。
- `webroot/` 是生成目录：不要手改、不要提交；改样式去 `webui/src/app.css`，改逻辑去 `webui/src/`。
- 所有面向用户的文案放 `LANGS.zh` / `LANGS.en`，渲染时通过 `LangCtx` 的 `t()` / `plural()` 取；禁止在组件里硬编码中文。两份字典必须同时补 key。
- 用户数据只用 `textContent` / Preact children 渲染；唯一使用 `dangerouslySetInnerHTML` 的地方是 `charts.js` 内部生成的数据驱动图表 SVG 字符串，不能把外部数据拼进 HTML。
- 顶栏三页统一高度（`.topbar-inner` 的 `min-height`），不要因为设置页没有刷新按钮就让它变矮；内容区与顶栏保留间距。
- 全局关闭 `-webkit-tap-highlight-color`、按钮 outline 和 user-select，避免 Android WebView 点按按钮出现原生蓝色浮层。

### 双语文案与格式

- Go：`uiText` / `uiKV`；WebUI：`LANGS` + `LangCtx`；shell：`L()` / `T`。内部日志、events、stderr 保持中文。
- 数值与复合单位之间留空格：`%d mAh`、`%d ℃`；中文百分号/量词紧跟数字：`健康 92%`、`循环 210次`。
- 英文描述风格：`Kernel health 96% | Measured health 90% (3600 mAh) | Cycles 210`；中文分隔符用“｜”，英文用 `" | "`。
- 日期展示按语言分流（`fmtTs` / `fmtRange`，英文 `Oct 24 12:03`）。

### 文档

- README.md 与 README_EN.md 的章节和标题锚点一一对应；语言导航行放在徽章区之后。
- 公开文档只描述本项目自身的行为、安装和使用方式，不写其他项目的来源归属或实现来源说明。
- 设计笔记、过程规格、调研报告放在本地工作区，不提交进仓库；`docs/` 只放按命名规范格式化的论文 PDF。

## 仓库结构

| 路径 | 用途 |
| :-- | :-- |
| `cmd/batteryd/` | Go daemon：采样、结算、容量估计、SQLite、description、`daemon` / `once` / `json` / `history` / `diag` 子命令 |
| `service.sh` | 开机等待 `sys.boot_completed=1` 后启动 daemon |
| `action.sh` | 管理器 Action：执行一次 `batteryd once`，透传退出码 |
| `customize.sh` | 刷入交互：两步按键（语言/确认，应用内与恢复模式一致）、保留旧 `data/`、设置二进制权限 |
| `webui/` | WebUI 源码：`index.html`、`public/boot.js`、`src/main.jsx` / `useDocPolling.js` / `useUiPrefs.js` / `i18n` / `core` / `charts` / `ui/` / `history/` / `assets/icons/` / `app.css` |
| `webui/public/` | 构建时原样复制的静态文件：`boot.js` |
| `webui/src/assets/icons/` | 静态 SVG 图标（导航、刷新），CSS mask 引用，构建时输出为独立文件 |
| `THIRD_PARTY_NOTICES.txt` | 随模块分发的第三方许可证（Preact MIT），放在模块根目录，不混入 WebUI 产物 |
| `webroot/` | `pnpm build` 生成的 WebUI 产物（模块 zip 使用），不提交 |
| `scripts/` | WebUI 构建守卫：离线扫描、gzip 体积检查、无头冒烟 |
| `package.json` / `pnpm-lock.yaml` / `vite.config.js` | WebUI 构建工具链与依赖锁定 |
| `module.prop` | 模块元数据；description 运行期由 batteryd 原子改写 |
| `module.yaml` / `module.json` / `update.json` | KernelSU Modules 展示/更新元数据 |
| `META-INF/com/google/android/` | 标准 Magisk 刷入桩，一般无需改动 |
| `.github/workflows/release.yml` | CI 构建、WebUI 守卫、KernelSU 校验、双变体打包与发布 |
| `.github/workflows/webui.yml` | PR/push 的 WebUI 构建、离线/体积守卫与无头冒烟 |
| `.github/workflows/go.yml` | PR/push 的 Go 守卫：gofmt / go vet / go test |
| `.release-baseline` | 上一版发布内容所在的 commit（更新日志基线），CI 发版后自动回写，不要手工改 |
| `README.md` / `README_EN.md` | 中英用户文档，必须同步 |
| `docs/` | 论文 PDF 库；过程文档不入库 |

- 运行时数据只在 `$MODDIR/data/` 内；历史查询按 90 天保留窗口取数，趋势点最多 120 个/类，事件正文按游标分页。

## WebUI 与宿主集成

### 取数协议

- 通过管理器官方 cbName 协议 `exec(cmd, '{}', 回调函数名)` 调用固定命令：`/data/adb/modules/CRecordEvolution/bin/batteryd json`、`... once`、`... history` 和 `... diag`。`diag` 只输出短文本（≤1800 字符），内容为设备型号/系统版本/root 管理器/电池节点/校准与最近会话摘要，外加最近一次校准或口径裁决的 detail（≤160 字，带 `(Nd)` 时效）与最后采样时间；日志按高价值关键词优先、启动行去重。**不得加入序列号、IMEI、MAC、账号等唯一标识**；所有 `getprop` 必须走 `diagAllowedProps` 白名单（仅 manufacturer/model/device/release/sdk/abi），白名单外的 key 直接返回空、不执行命令，新增字段前先确认不是设备唯一标识。WebUI 设置页「复制诊断」按钮执行它并写入剪贴板（失败时回退为可长按选择的只读文本框）。
- 历史页使用 `history --days 7|30|90 --limit 20 --type all|charge|discharge [--before TS]`：服务端在范围内合并充电/放电事件，按时间倒序游标分页，并返回范围内摘要与趋势点；`json` 命令保持原样，仅供概览/设置使用。
- 有 15s 超时和 `ksu` → `APatch`/`apatch` 的宿主级联回退；普通浏览器没有 `ksu` 时显示 gate 页。
- 命令必须是常量或由固定模板拼接；`history` 的参数只接受数字与固定枚举，不要把用户输入、远端数据或任意字符串拼进 shell 命令。

### Edge-to-edge / 系统栏

- `<head>` 使用根相对路径请求 `/internal/insets.css`；KSU/APatch 的 WebView asset loader 会拦截该请求，自动把 WebView 铺到系统栏下、把宿主 padding 置 0，并下发 `--window-inset-*`。KSU 新版还会动态注入 `--safe-area-inset-*`。
- 页面在 head 里尽早调用 `ksu.enableEdgeToEdge(true)`，旧接口 `enableInsets(true)` 兜底；`main.jsx` 挂载时、350ms 和 `load` 时再各调一次，兼容接口就绪较晚的宿主。
- CSS 变量回退链：`--safe-area-inset-*` → `--window-inset-*` → `env(safe-area-inset-*)` → 0。**不要删除 insets.css 请求，也不要把 inset 写死。**
- 不要把宿主没 edge-to-edge 的情况当成 bug：KSU ≤2.1.2 / APatch ≤11142 本来就会内缩，页面不能再重复加 padding。
- **不要画 `#sysbar` / `#navbar` 对比带**：`prefers-color-scheme` 与页面主题判断不一致时会变成上下黑/白条。inset 区域交给页面背景铺满；MMRL 若提供 legacy `setLightStatusBars()`，用 `syncStatusBarStyle()` 同步图标明暗。

### 三页与设置项

- 概览：健康度主卡（含主卡内、容量行下方的温度行 `temp_c`，缺失即整行省略）、容量/循环/估算/内阻/趋势/可信度、估算趋势图。`contentFP` 按 0.1℃ 粒度把 `temp_c` 纳入内容指纹，否则温度变了不重绘。趋势图的 `recent` 与 `trend_mah_per_week` 共用同一份 estimates 窗口（最近 200 点）；趋势显著时 JSON 附带 `trend_from_mah`/`trend_to_mah`（拟合线在窗口首/末点的容量），前端直接用这两个端点画虚线——不要只下发斜率让前端拿首个实际点外推，那样虚线与数据的时间窗/锚点不一致，会显示成「虚线方向和数据相反」。
- 历史：摘要统计；`记录` 视图按日期分组的时间轴（充电/放电，范围与类型筛选合并为一行 chip，7/30/90 天 + 全部/充电/放电，分页“加载更早”）；`趋势` 视图的容量与充电会话大图 + 放电/静置/CCCT/ICA 小多图，小图点按展开；大图与小图展开后的大图都支持点按数据点读数值并高亮选中点（`charts.js` 的绘图函数接受可选选中索引，只能拼内部数值）；记录详情渐进展开。
- 设置：外观（主题/语言/底栏）、手动刷新、数据信息（含采样节点与电流刻度诊断，来自 JSON 顶层 `calib`；末尾附「复制诊断」按钮）、关于（品牌卡 + 版本/通道/许可/第三方信息，版本来自 JSON 顶层 `version`）。`json` 的 `sessions` 数组只是最近 10 条，设置页「会话数」必须用顶层 `sessions_n` 总数；会话条目带 `start_cap`/`end_cap`，趋势卡与时间轴的百分比是 **SOC 百分点变化**（如 70%→97% 即 27 个点），不是容量增长。
- 底栏支持悬浮胶囊 / 传统贴底（`cre_barmode`，默认悬浮）；主题 `cre_theme` 为 `auto|light|dark`；语言 `cre_lang`。
- 切页保存/恢复滚动位置；设置页隐藏顶部刷新按钮，换成页内“立即刷新”。

### 语言文件

- 优先级：`data/lang`（安装器/WebUI 写入，启动时 `cat` 读取并覆盖自动检测）> localStorage `cre_lang` > `navigator.languages`。
- 设置页切换语言时同时原子写 `data/lang`（临时文件 + `mv`），再执行一次 `batteryd once` 刷新模块描述；连续快速切换要排队，避免旧写入覆盖新选择。

## 领域不变量与已知坑

### 采样与会话结算

- 采样间隔：充电期 15s，其余 60s；满电后不回封账，CV 尾段由 `tickTailCharge` 延续累计，待电流停歇去抖后结算。
- 会话电量双口径：电量计差分（`charge_counter`，结算估算首选）与电流积分（`avgI`、吞吐、样本口径）。两者相差 3 倍以上时用容量参考值（设计容量，缺失时用 `charge_full`）做 0.5~1.5 倍窗口裁决：只有「电量计隐含容量在窗内、积分在窗外」才保留电量计，其余情况维持旧行为（回退积分）；3 倍失配导致裁决时写 `counter_scale` 事件（差 3 倍以内不写）。
- `chargeSuppressed`：满电后系统高负载时内核仍报 Charging 且电流为正，但端电压相对充电峰值回落超过 `30mV × 电芯数`，视为充电器直供系统的假电流，不计入会话电量与循环吞吐；该门控只作用于电流积分口径。

### 容量估计、校准与趋势

- 构建注入 `main.channel` 选择 stable/ml Estimator。
- 中段校准 `midwin`：充电样本按 1% 显示百分点分桶、按穿越次数归一，聚合 `[30%, 90%)` 窗口；每日至多一次按 1/10 权重并入 EMA。区间电量优先取 `samples.cc`（列名历史遗留，实为 charge_counter 读数 µAh）的电量计差分；样本里有 counter 读数时，缺 counter 的旧区间直接跳过，只有整批无 counter 才回退电流积分。隐含值必须过 `capacityInWindow` 门控，最近一次采信值存 kv `mid_implied_ua`。
- 放电通道 `discharge`：读 `charge_counter` 差分累计，转入充电即结算；放电样本按 5 分钟降采样。结算门控为掉幅 ≥30 点、起始 ≤95%、过 `capacityInWindow`，按 1/10 权重并入 EMA，采信值存 `dis_implied_ua`。该通道不经 `current_now`，是四路容量口径中最稳的一路。
- 质量哨兵 `quality`：ema/mid/dis/full 四路互检，互差 >40% 判不一致；结论翻转才写 `quality_warn` 事件。可用路数 <2 时整体省略 `quality` 字段。mid/dis 的隐含值带时间戳（kv `mid_implied_ts`/`dis_implied_ts`），超过 90 天保留窗口就不参与互检（否则半年没放电的设备会拿旧值永久误报）；缺 ts 的旧库首次读取按「当天」回填并给一次宽限。时效判定在结算侧 `checkQuality`（会写 ts、落 `quality_stale` 事件）与 `stats()` 的 JSON 出口（只读过滤）共用同一个纯函数，改一处必须同步另一处。只记事件，不改变 JSON 契约。
- 趋势 `trend`：三态 `insufficient|stable|significant`，显著性用 Mann-Kendall 点对符号检验（双侧 5%）。R² 门控已废弃，单次估算噪声远大于真实周衰减。

### sysfs 与单位

- `NormCurrentUAWithFull` 以 `charge_full` 的**量纲**判别电流单位：≥100000 只可能是 µAh，同驱动 `current_now` 已是 µA 直通；以 mAh 计则 ×1000；无锚点退化为幅值启发式；最后用 25A 物理上界兜底。
- **不要拿 `charge_full` 当幅值阈值**：曾按 `|I| < charge_full/10` 判成 mA，把 0.6A 以下的真实 µA 读数整批 ×1000，污染采样、积分和校准。
- `current_now` 的量纲可能随内核/厂商不同：一加 Ace 3 等欧加系机型报 mA，而同一节点的 `charge_counter` / `charge_full` 报 µAh。旁证：公开的 OnePlusOSS SM8550/Ace 3 分支 `oplus_charger.c` 对 `CHARGE_FULL`（mAh→µAh）、`VOLTAGE_NOW`（mV→µV）都做了 ×1000 换算，`CURRENT_NOW` 却直接透传 gauge 的 mA 读数；同类 gauge 驱动的量程常量（如 `CURR_MAX 25000`）也按 mA 语义。生产路径一律走 `Pipeline.currentUA`（内部 `normalizeCurrentUA`：已学习刻度优先，未校准时退回 `NormCurrentUAWithFull` 锚点），**禁止把原始读数当 µA 直接积分**，也禁止只凭 `charge_full` 的量纲就断言电流单位。
- 运行时自校准：充电会话结算时用「电量计差分电量 vs 会话期原始电流读数积分（rawUAs）」反推 µA / 原始单位；**不能用已被当前刻度/锚点缩放过的 `accUAs` 反推**，否则已学 ×1000 会在下一会话被算成 ×1、与真实刻度来回震荡（回归断言见 `TestIssue6MilliampCurrentCalibration` 的第二会话段）。比值须落在 10 的整数次幂（10⁻³~10³）±0.4 decade（最多 2.5 倍偏差）内才采信（且仅在结算已采信电量计口径后学习）；容差 <0.5 保证相邻 10 的幂吸附区不重叠，×1 与 ×1000 不可能混判。容差放到 0.4 是 2026-10 一加 13 实机反馈倒逼的：双电芯等效容量口径下电流积分只有电量计的 52%（电量计 1074mAh vs 原始积分 556mAh，比值 1.93×10³），0.15 decade 会把这类设备永久拒在校准之外，avgI/实时电流恒为 0。采信值写 kv `cur_scale`，之后所有 `current_now` 读数按该刻度归一；事件 `cur_cal` 记录刻度落盘/变化（含原始比值），`cur_cal_fail` 记录写库失败或比值不达吸附条件（带原始比值，供诊断定位），`counter_scale` 仍记录结算时的口径裁决。允许覆盖旧刻度：系统 OTA 改变 `current_now` 单位后，下一个采信会话会自动纠正，不会永久卡住。
- `sessions.cc` 是会话期电量计差分电量（µAh，新增列）：存结算时口径裁决采信的电量计增量——即使该会话随后因温度/涨幅等门控被拒也照存（历史页据此展示真实量级的估算），`0` 表示无电量计或裁决未采信（回退积分）。历史页「估算」优先由它折算，缺 `cc` 时才回退电流积分。`sessions.ua` 仍是积分口径（µA·s），两者不要混用。
- `NormTempC` 截断 `[-40, 80]°C`；`readDTBatteryCapacity` 在 `charge_full_design` 缺失时从设备树读 `vivo,bat-capacity-mah`，绝对路径优先。
- `current_now` 带符号读取（放电为负），其余节点严格非负。

### 时间与存储

- 设备上 Go 的 `time.Now()` 可能返回 UTC（无 tzdata）。面向用户的时间戳一律走 `localNow()`（getprop `persist.sys.timezone` → 文件兜底 → UTC），内嵌 `time/tzdata`；Pipeline 注入时钟同样传 `localNow`。
- SQLite 十表：kv/sessions/estimates/resistance/rest_points/events/samples/ccct/ica_peaks/discharge；90 天清理。WAL + `wal_autocheckpoint=256` + 每日 `wal_checkpoint(TRUNCATE)`。
- 旧库升级会做幂等修复：`repairSampleUnits` 按 25A 上界把误判样本 ÷1000，kv 标记 `samples_unit_repair`；给 `samples` 补 `cc` 列，给 `sessions` 补 `cc` 列（会话期电量计差分 µAh，旧行默认 0）。
- `module.prop` 的实时 description 用临时文件 + rename 原子写回，不要改成 sed。

### 关键数值与口径（勿随意改）

- 双电芯判定阈值是 **5V**，不是 4.5V：高压单电芯截止 4.45~4.53V，4.5V 会误判并让电压窗口翻倍、CCCT/ICA 全哑；双电芯串联最低约 6.8V。
- 缩放函数 `initCCCTVoltage` / `initICAVoltage` 必须无条件赋值以支持复位，否则测试之间全局状态互相污染。
- **不要对 `charge_full` 乘电芯数**：不同内核口径不同，若已是整包会翻倍；`healthPct(full*cellCount, design*cellCount)` 这类同乘是 no-op。
- **不要拿 `current_now` 积分当绝对容量口径**：实机同 SOC 区间充电积分只有放电差分的 82~95%，放电差分与内核 `charge_full` 只差 0.8%。会话结算与中段校准首选 `charge_counter` 差分，积分只在 counter 不可用/刻度异常时兜底。

### 已知局限（有意取舍，勿当 bug）

- 电流单位启发式在涓流 <10mA 时可能误判；RLS 无遗忘因子、P 矩阵长期膨胀是潜伏项；`FindNode` 全树兜底只在缺节点时触发。
- 节点缺失时描述/JSON/once 按可用字段降级，不整体失败。
- 放电会话只在 `status=Discharging` 时累计；旁路供电机型（status=Full 但实际放电）会漏记。
- 放电隐含容量以“显示掉幅”为分母，顶部区间显示被压缩，绝对值偏高，只作观测，不进估算通道。
- 电流刻度自校准在充电会话结算时才发生：学到新刻度时会按新刻度重算该会话的积分电量、平均电流与倍率（`sessions.ua`/`avg_i`/`c_rate`），并把该会话区间的 `samples.ua`、`resistance.mo` 按比例回填、修正已有 `ica_peak_base`（避免首个校准会话用旧刻度把 ICA 基准带偏 1000×，事件 `cur_backfill`/`cur_backfill_fail`/`cur_backfill_skip`）；仅当旧有效刻度可确定（已学刻度 >0，或 `anchorScaleUA(charge_full)` 为 1/1000）时才回填，无锚点机型仍保留旧刻度并记事件。回填只覆盖本会话的 `samples`/`resistance` 与 ICA 基准，`charged_ua_total` 吞吐与历史行不回溯；已发布库中既有的坏基准无法自动识别，只能靠新学习路径不再污染；没有 `charge_counter` 的机型缺少标尺，无法自校准，只能沿用 `charge_full` 量纲初判（有 `charge_counter`、没有 `charge_full` 的机型仍可校准：rawUAs 不依赖锚点）。

## 安全注意事项

- WebUI 的 `exec` 在 root 下执行，命令必须是固定字符串或固定模板；**绝不要**把用户输入、语言值、模块描述、数据库内容拼进 shell。
- 不要引入远程脚本、CDN、遥测或上传；数据只留在本机。运行时依赖由 lockfile 固定版本并随产物附许可证（见根目录 `THIRD_PARTY_NOTICES.txt`）。
- 不要给 `webroot` 加 chmod/set_perm；安装器只处理 `bin/batteryd`。
- 不要提交 `bin/`、`dist/`、生成 zip、`data/`、`*.exe` 或任何密钥/签名文件。
- 处理图表 SVG 时只使用内部生成的数据；不要把宿主/远端字符串注入 `dangerouslySetInnerHTML`。
- 修改安装器或 service 时，先确认不会删除 `$MODDIR/data/battery.db`；升级必须保留旧 `data/`。

## 发布流程

0. 仓库 Settings → Releases 必须已开启 **Enable release immutability**；未开启时 KernelSU Modules 会报 `release-not-immutable`，该版本只能作废。
1. 同步修改 `module.prop` 的 `version` 与 `versionCode`，必须严格递增，**不得复用历史值**。
2. 提交到 main（只提交源码；`webroot/` 是生成目录，不入库）。
3. `git tag vX.Y.Z && git push origin main --tags`。CI 会：校验标签与 version 一致 → go vet + go test → Go 构建 → `pnpm install --frozen-lockfile` + `pnpm run verify` 构建并守卫 WebUI → 打包 stable/ML 两个变体 → KernelSU 官方校验器 dry-run → 创建 draft Release 并上传产物 → 发布（immutable）→ 回写 `update.json` 与 `.release-baseline`。
4. 标签必须与 `module.prop` 的 `version` 一致（如 v1.2 ↔ version=1.2），否则 CI 直接失败。
5. 更新日志基线：根目录 `.release-baseline` 记录「上一版发布内容所在的 commit」，`Generate changelog` 用它算 `git log` 区间，发版后由 CI 回写，不要手工改。**不能**改用 `git describe --tags` 找上一版：tag 可能与 main 历史脱节（v1.3.6 的 tag 就不在 main 祖先上），临时测试 tag（如 `issue6-test-*`）也可能落在祖先链上，两种情况都会让更新日志只剩几条或重复整版内容。
6. 发版 tag 必须落在 main 的历史上；临时测试 tag 用完即删（`git push --delete origin <tag>`），不要长期留在 origin 干扰任何基于 tag 的工具。
7. 资产名：`CRecordEvolution-{versionCode}-{version}.zip` / `CRecordEvolution_ML-{versionCode}-{version}.zip` / `changelog.txt`；`module.yaml` 的 `release.assetPattern` 只匹配 stable。两个 zip 都包含根目录 `THIRD_PARTY_NOTICES.txt`。

## 提交前检查清单

- [ ] 已读相关源码和 README/AGENTS 约定，没有覆盖用户未提交的改动。
- [ ] 改动的 Go 逻辑有测试；`go test ./...`、`go vet ./...` 通过。
- [ ] shell 改动通过 `bash -n`，并说明无法在真机验证。
- [ ] WebUI 改动通过 `pnpm run verify` 与 `pnpm run smoke`（或真机/模拟宿主），三页/设置/图表/分页都检查过。
- [ ] 没有把 `webroot/`、`node_modules/`、`artifacts/` 加入提交。
- [ ] 新增 UI 文案同时更新 zh/en；新增 JSON 字段两端同步。
- [ ] 需要发布的版本才改 `module.prop` 版本号；不提交 `bin/`、`dist/`、zip。
- [ ] 本地打包时 `.sh`、`module.prop`、`META-INF` 是 LF，不是 CRLF。
- [ ] 没有提交密钥、签名、数据文件或本地调试产物。
- [ ] 未经用户明确要求，不自行 `git commit`、打 tag 或触发发布。
