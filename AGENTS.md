# AGENTS.md

本文件面向在本仓库工作的编码代理，说明项目目标、构建/测试方式、代码约定、领域不变量和发布流程。面向用户的介绍、安装步骤和功能说明在 [README.md](README.md) / [README_EN.md](README_EN.md)，不在这里重复。

## 项目概览

ChargingRecord Evolution 是一个 Android 模块（id=`CRecordEvolution`），用 Go 二进制 `batteryd` 读取电池 sysfs、结合充电会话估算健康度与容量，并把结果写入管理器的模块描述；模块自带一个三页 WebUI（概览 / 历史 / 设置），数据保存在本机 SQLite。

**全项目有且仅有两种叫法**：`CRecordEvolution`（模块 id、安装路径与数据目录、资产名前缀、代码标识符）与 `ChargingRecord Evolution`（展示名：`module.prop`/`module.yaml` 的 `name`、README 标题、WebUI 标题与关于卡、发布标题）。文档、UI、工作流新增文案只能用这两种，不要写 `ChargingRecord`、`Charging Record`、`ChargingRecord Evolution ML` 之类的变体；ML 变体与 stable 同名，只靠 description 前缀 `[ML实验版]` 和资产名区分，**不改 `name`**。不属于展示名的历史遗留保持原样：旧模块 id `Charging_Record`（README 升级提示、`describe_test.go` 夹具）与 `go.mod` 的模块路径 `charging_record`。

- 业务逻辑全部在 `cmd/batteryd/`；`service.sh`、`action.sh`、`customize.sh` 只是 POSIX sh 引导/交互壳，由 root 管理器执行。
- WebUI 源码在 `webui/`（Preact + Vite，ES module/JSX），构建到 `webroot/`（生成目录，不提交）。
- 支持 KernelSU / APatch / MMRL 的 WebUI；Magisk 通过标准 META-INF 刷入桩安装；只发布 arm64 静态二进制。
- 纯本机运行：无网络请求、上传或遥测。

### 两种构建变体

| 变体 | 构建差异 | 产物 |
| :-- | :-- | :-- |
| stable | 默认构建（`channel=stable`） | `CRecordEvolution-{versionCode}-{version}.zip` |
| ML 实验版 | 注入 `-X main.channel=ml`；CI 只在 description 前加 `[ML实验版]`、删 `updateJson`，`name` 不变 | `CRecordEvolution_ML-{versionCode}-{version}.zip` |

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
pnpm run format   # Prettier 写回 webui/、scripts/、vite.config.js；format:check 只检查不写
```

- Go 版本以 `go.mod` 为准；SQLite 用纯 Go 的 `modernc.org/sqlite`（无 CGO，Windows 可直接跑测试）；Node/pnpm 版本由 `package.json` 固定。
- `bin/batteryd`、`webroot/`、`node_modules/`、`artifacts/` 都不提交。Vite root 在 `webui/`，入口 `webui/index.html`，产物固定到 `webroot/`；`pnpm run dev` 起本地开发服务器。
- **产物两条硬约束**：完全离线（CI 扫描 `fetch`/`XMLHttpRequest`/远程 URL/远程 import）与 gzip ≤50KB（`scripts/check-size.mjs`）；改依赖或大段代码后先跑 `pnpm run verify`。`build.modulePreload.polyfill` 必须保持关闭（它会注入 `fetch`）。

### WebUI 冒烟检查

改 `webui/` 后先 `pnpm run build`，再 `pnpm run smoke`（起静态服务、注入 mock `ksu.exec`、截图写到 `artifacts/smoke/`），或 `pnpm run dev` 手测。至少检查：三页切换且无 console 报错、`assets/*` 与 `boot.js` 无 404；设置页主题/语言/底栏；历史页 7/30/90 天与筛选、滚动加载、详情与趋势小图展开；无 `ksu` 时显示 gate 页而非白屏。

### 不能本地验证与测试要求

无 Android 真机：root 管理器执行脚本、KSU/APatch/MMRL WebView 的 `exec`/edge-to-edge/CSP、Magisk 安装升级流程只能通读推演；涉及这些路径必须声明「未在真机验证」并给最小验证步骤。

- 改 Go 逻辑必须补/改测试，`go test ./...` 全绿、`go vet ./...` 通过。
- 改 shell 至少 `bash -n customize.sh action.sh service.sh`；不要用 bash 语义「修正」POSIX sh，不在 Windows 跑安装流程。改安装器要推演「应用内安装与恢复模式同样读音量键」两步按键，以及读不到 `getevent`/`timeout` 时自动安装、不能误取消。
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
- 语言选择步固定中英双语、一行一项（标题、音量+、音量-、超时说明）；选定语言后其余步骤用该语言渲染。
- 应用内安装与恢复模式用同一套两步按键（选语言 → 确认安装）；**不按 BOOTMODE/管理器类型跳过**（2026-10 实机确认 LSPosed/Zygisk 系在 App 内也能读音量键）。只有读不到 `getevent`/`timeout` 时才自动安装，绝不能误取消。
- `read_key` 只采纳 `DOWN`，忽略 `UP`/`SYN`（否则一次按键被当成两次）。回归：按一次取消、按两次才安装。
- `SKIPUNZIP=0` 时已解压到 `$MODPATH`，不要重复 `unzip`，仅缺 `module.prop` 时兜底一次。
- 只发 arm64：用 `$ARCH` 门控（缺失跳过），非 arm64 直接失败。
- 交互文案走 `T`/`L`，语言探测顺序与 `i18n.go` 一致；不把逻辑改动混进文案替换。
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
- 顶栏三页统一高度（`.topbar-inner` 的 `min-height`）；全局关闭 `-webkit-tap-highlight-color`、按钮 outline 与 user-select。

### 双语文案与格式

- Go 用 `uiText`/`uiKV`；WebUI 用 `LANGS` + `LangCtx` 的 `t()`/`plural()`（两份字典同时补 key，组件禁止硬编码中文）；shell 用 `L()`/`T`。内部日志、events、stderr 保持中文。
- 数字与复合单位留空格（`%d mAh`、`%d ℃`）；中文百分号/量词紧跟数字（`健康 92%`、`循环 210次`）。英文描述用 ` | `、中文用「｜」；日期按语言分流（`fmtTs`/`fmtRange`）。

### 文档

- README.md 与 README_EN.md 章节/锚点一一对应，语言导航行在徽章区之后；公开文档只写本项目行为，不写来源归属。
- 设计笔记、过程规格、调研报告放本地工作区不提交；`docs/` 只放按命名规范格式化的论文 PDF。

## 仓库结构

| 路径 | 用途 |
| :-- | :-- |
| `cmd/batteryd/` | Go daemon：采样、结算、容量估计、SQLite、description、各子命令 |
| `service.sh` / `action.sh` / `customize.sh` | 启动壳 / Action 跑一次 `once` / 刷入两步按键交互 |
| `webui/` → `webroot/` | WebUI 源码（Vite）→ 构建产物（进 zip，不提交） |
| `scripts/` | 离线扫描、gzip 体积检查、无头冒烟 |
| `module.prop` / `module.yaml` / `module.json` / `update.json` | 模块元数据与 KernelSU 展示/更新（`assetPattern` 只匹配 stable） |
| `META-INF/...` / `THIRD_PARTY_NOTICES.txt` | Magisk 刷入桩 / 随包第三方许可（Preact MIT） |
| `.github/workflows/` | release（双变体发布）/ webui（守卫+冒烟）/ go（gofmt+vet+test） |
| `.release-baseline` | 更新日志基线（上一版发布内容 commit），CI 回写，勿手改 |
| `README.md` / `README_EN.md` / `docs/` | 中英文档必须同步 / 论文 PDF 库 |

运行时数据只在 `$MODDIR/data/` 内；历史查询按 90 天窗口取数，趋势点 ≤120 个/类，事件正文游标分页。

## WebUI 与宿主集成

### 取数协议

- 通过管理器 cbName 协议 `exec(cmd, '{}', 回调函数名)` 调用固定命令：`/data/adb/modules/CRecordEvolution/bin/batteryd json|once|history|diag`。
- `diag` 只输出短文本（≤1800 字符）：机型/系统/root 管理器/电池节点/校准与最近会话摘要 + 最近一次校准或口径裁决 detail（≤160 字、带 `(Nd)` 时效）+ 最后采样时间；日志按关键词优先、启动行去重。**不得含序列号/IMEI/MAC/账号**；`getprop` 必须走 `diagAllowedProps` 白名单（manufacturer/model/device/release/sdk/abi），白名单外直接返回空。设置页「复制诊断」执行它写剪贴板（失败回退只读文本框）。
- 历史页用 `history --days 7|30|90 --limit 20 --type all|charge|discharge [--before TS]`：服务端合并充电/放电事件、按时间倒序游标分页，返回摘要与趋势点；`json` 保持原样供概览/设置。
- 15s 超时；`ksu` → `APatch`/`apatch` 级联回退；无 `ksu` 显示 gate 页。命令必须是常量或固定模板拼接；`history` 参数只接受数字与固定枚举，不得拼用户输入或远端数据。

### Edge-to-edge / 系统栏

- `<head>` 用根相对路径请求 `/internal/insets.css`；KSU/APatch WebView 会拦截并铺满系统栏、下发 `--window-inset-*`（新版 KSU 另有 `--safe-area-inset-*`）。
- head 里尽早 `ksu.enableEdgeToEdge(true)`，旧接口 `enableInsets(true)` 兜底；挂载时、350ms、`load` 各再调一次。
- 回退链：`--safe-area-inset-*` → `--window-inset-*` → `env(safe-area-inset-*)` → 0。**不删 insets.css 请求，不写死 inset**。
- 宿主没 edge-to-edge 不是 bug（KSU ≤2.1.2 / APatch ≤11142 本就内缩），页面不要再加 padding。
- **不要画 `#sysbar`/`#navbar` 对比带**（主题判断不一致会出黑白条）；inset 区交给页面背景；MMRL 的 legacy `setLightStatusBars()` 用 `syncStatusBarStyle()` 同步。

### 三页与设置项

- 概览：健康度主卡（容量行下方温度行 `temp_c`，缺失整行省略）、容量/循环/估算/内阻/趋势/可信度、估算趋势图。`contentFP` 按 0.1℃ 纳入 `temp_c`；`recent` 与 `trend_mah_per_week` 共用最近 200 点窗口；趋势显著时 JSON 给 `trend_from_mah`/`trend_to_mah`（窗口首/末点容量），前端直接连两端点画虚线，不要下发斜率让前端外推。
- 历史：摘要统计；记录视图按日期分组时间轴（范围+类型合并成一行 chip：7/30/90 天、全部/充电/放电，分页「加载更早」）；趋势视图为容量与充电会话大图 + 放电/静置/CCCT/ICA 小多图（小图点按展开）；大图与展开图都支持点按数据点读数并高亮（`charts.js` 接受可选选中索引，只拼内部数值）；详情渐进展开。
- 设置：外观（主题/语言/底栏）、手动刷新、数据信息（含采样节点与电流刻度诊断，来自 JSON 顶层 `calib`，末尾「复制诊断」）、关于（品牌卡 + 版本/通道/许可/第三方）。`json` 的 `sessions` 只有最近 10 条，「会话数」必须用顶层 `sessions_n`；会话条目 `start_cap`/`end_cap`，趋势卡与时间轴百分比是 **SOC 百分点变化**（70%→97% 是 27 个点），不是容量增长。
- 底栏悬浮胶囊 / 贴底（`cre_barmode`，默认悬浮）；`cre_theme` 为 `auto|light|dark`；`cre_lang`。切页保存/恢复滚动；设置页隐藏顶栏刷新，用页内「立即刷新」。

### 语言文件

- 优先级：`data/lang`（安装器/WebUI 写入，启动读取并覆盖自动检测）> localStorage `cre_lang` > `navigator.languages`。
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
- 运行时自校准：充电结算时用「电量计差分 vs 会话期原始电流积分 `rawUAs`」反推 µA/原始单位，**不能用已缩放的 `accUAs` 反推**（否则 ×1000 与 ×1 来回震荡，回归见 `TestIssue6MilliampCurrentCalibration` 第二会话段）。比值须落在 10 的整数次幂 ±0.4 decade（≤2.5 倍偏差；<0.5 保证吸附区不重叠）且结算已采信电量计口径才学习；采信值写 kv `cur_scale`，事件 `cur_cal` 记落盘/变化、`cur_cal_fail` 记写库失败或比值不达标、`counter_scale` 记口径裁决。允许覆盖旧刻度（OTA 改单位后下个会话自动纠正）。
- `sessions.cc` 存结算时采信的电量计增量 µAh（被门控拒绝也照存；0=无或未采信）；历史页「估算」优先由它折算，缺才回退积分。`sessions.ua` 仍是积分口径，两者不混用。
- `NormTempC` 截断 `[-40, 80]°C`；`readDTBatteryCapacity` 在 `charge_full_design` 缺失时读设备树 `vivo,bat-capacity-mah`（绝对路径优先）；`current_now` 带符号读取（放电为负），其余节点严格非负。

### 时间与存储

- 设备 `time.Now()` 可能是 UTC：面向用户的时间戳一律 `localNow()`（getprop `persist.sys.timezone` → 文件兜底 → UTC，内嵌 `time/tzdata`）；Pipeline 注入时钟同样用 `localNow`。
- SQLite 十表：kv/sessions/estimates/resistance/rest_points/events/samples/ccct/ica_peaks/discharge；90 天清理。WAL + `wal_autocheckpoint=256` + 每日 `wal_checkpoint(TRUNCATE)`。
- 旧库升级做幂等修复：`repairSampleUnits` 按 25A 上界把误判样本 ÷1000（kv `samples_unit_repair`）；`samples`/`sessions` 补 `cc` 列（旧行默认 0）。
- `module.prop` 的实时 description 用临时文件 + rename 原子写回，不要改成 sed。

### 关键数值与口径（勿随意改）

- 双电芯判定阈值是 **5V**：4.5V 会误判高压单电芯（截止 4.45~4.53V），导致电压窗口翻倍、CCCT/ICA 全哑。
- `initCCCTVoltage`/`initICAVoltage` 必须无条件赋值以支持复位，否则测试间全局状态污染。
- **不要对 `charge_full` 乘电芯数**（不同内核口径不同，已是整包会翻倍）。
- **不要拿 `current_now` 积分当绝对容量口径**（实机同 SOC 区间只有放电差分的 82~95%）；结算与中段校准首选 `charge_counter` 差分，积分只在 counter 不可用/刻度异常时兜底。

### 已知局限（有意取舍，勿当 bug）

- 电流单位启发式在涓流 <10mA 可能误判；RLS 无遗忘因子、P 矩阵长期膨胀是潜伏项；`FindNode` 全树兜底只在缺节点时触发。
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
5. 更新日志基线用 `.release-baseline`（上一版发布内容所在 commit），CI 发版后回写，勿手改；**不能**改用 `git describe --tags` 找上一版（tag 可能与 main 脱节，测试 tag 会干扰）。
6. 发版 tag 必须落在 main 历史上；临时测试 tag 用完即删。
7. 资产名：`CRecordEvolution-{versionCode}-{version}.zip`、`CRecordEvolution_ML-{versionCode}-{version}.zip`、`changelog.txt`；`assetPattern` 只匹配 stable；两个 zip 都含 `THIRD_PARTY_NOTICES.txt`。

## 提交前检查清单

- [ ] 已读相关源码和 README/AGENTS 约定，没有覆盖用户未提交的改动。
- [ ] Go 改动有测试；`go test ./...`、`go vet ./...` 通过。
- [ ] shell 改动通过 `bash -n`，并说明无法在真机验证。
- [ ] WebUI 改动通过 `pnpm run verify` 与 `pnpm run smoke`（或真机/模拟宿主），三页/设置/图表/分页都检查过。
- [ ] 没有把 `webroot/`、`node_modules/`、`artifacts/` 加入提交。
- [ ] 新增 UI 文案同时更新 zh/en；新增 JSON 字段两端同步。
- [ ] 需要发布的版本才改 `module.prop` 版本号；不提交 `bin/`、`dist/`、zip。
- [ ] 本地打包时 `.sh`、`module.prop`、`META-INF` 是 LF。
- [ ] 没有提交密钥、签名、数据文件或本地调试产物。
- [ ] 未经用户明确要求，不自行 `git commit`、打 tag 或触发发布。
