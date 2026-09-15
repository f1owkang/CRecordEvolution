# AGENTS.md

Magisk 模块「ChargingRecord Evolution」（id=`CRecordEvolution`，作者 `f1owkang、不会梦游的鱼、阿巴酱`）：Go 二进制 `batteryd` 读取 sysfs 并结合充电会话库仑积分估算电池健康度，结果显示在管理器的模块描述里。业务逻辑全部在 Go（`cmd/batteryd/`）；仓库里的 `service.sh` / `action.sh` / `customize.sh` 只是 POSIX sh 引导/交互壳，在 Android 设备上由 root 管理器执行，本机无法运行验证。

## 构建

- 本机验证：`go test ./...`（Windows 可直接跑，modernc.org/sqlite 纯 Go 无 CGO）。
- 设备端构建命令（CI 同款，android/arm64 静态二进制）：
  `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o bin/batteryd ./cmd/batteryd`
- `bin/batteryd` 是 CI 打包进 zip 的产物，不要提交进仓库；本地构建残留（如 `batteryd.exe`）同样勿提交。

## 文件职责

- `cmd/batteryd/` — 全部业务逻辑：sysfs 探测与单位判别、60s 采样管道与会话结算、stable/ml 双通道 Estimator、SQLite 存储与 90 天清理、description 组装与原子写回、`daemon | once | json` 子命令分发。算法通道由构建注入 `-X main.channel=ml` 区分。分模块要点：
  - `pipeline.go` — Tick 状态机与累计。充电期 15s / 其余 60s；满电后不回封账，CV 尾段由 `tickTailCharge` 延续累计，待电流停歇去抖结算。**满电后电压回落门控**（`chargeSuppressed`）：满电后系统高负载时内核仍报 Charging 且电流为正，但端电压较充电峰值回落 >30mV×电芯数，实为充电器直供系统的假电流，不计入会话电量与循环吞吐。
  - `trend.go` — 容量趋势三态（insufficient/stable/significant）。显著性用 Mann-Kendall 点对符号检验（双侧 5%）；R² 门控已废弃——单次会话估算噪声远大于真实周衰减，实测 R² 长期为负、等于趋势永不可达。
  - `midwin.go` — 中段分窗容量校准：充电样本按 1% 显示百分点分桶、按穿越次数归一，聚合 [30%,90%) 窗口的隐含容量，每日至多一次按 1/10 权重并入 EMA。收益是不满充也能出容量估计，且避开顶部压缩段。
  - `discharge.go` — 放电会话记录：读电量计 `charge_counter` 差分累计（芯片内积分，免疫负载混叠），转入充电即结算。放电样本按 5 分钟降采样落 `samples`（带符号电流，放电为负）。
- `service.sh` — 开机延迟引导：等 `sys.boot_completed=1`（2s 轮询）后 `exec "$MODDIR/bin/batteryd" daemon`；首刷重试（10 次 × 30s）在 Go 内。
- `action.sh` — Action 按钮：执行 `bin/batteryd once` 并透传退出码，失败提示查 events 表。
- `customize.sh` — 刷入交互脚本：打印设备信息后音量键确认（音量+ 安装 / 音量- abort），解压后 `set_perm "$MODPATH/bin/batteryd" 0 0 0755`。模块元信息（name/version/author）直接从 `$MODPATH/module.prop` 读取（不依赖安装器注入的 `$MODNAME/$MODVERSION` 等变量，各管理器环境差异大）；升级时把旧模块 `data/` 复制进新 `$MODPATH/data`，因为 Magisk/KSU 的 staged update 重启会整体删除旧模块目录、否则 `data/battery.db`（学习记录）会丢。
- `webroot/index.html` — KSU/APatch/MMRL WebUI 单文件仪表盘（内联 CSS/JS 零依赖），取数走管理器官方 cbName 协议 `exec(cmd, '{}', 回调函数名字符串)` 并带 15s 超时与级联回退，调 `/data/adb/modules/CRecordEvolution/bin/batteryd json`。权限由安装器接管，**不要给它加 chmod/set_perm**。
- `module.prop` — 模块元数据。默认 `description=Magisk模块，通过读取系统容量估算电池健康度`；该行由 batteryd 运行期改写为实时电池健康数据（临时文件 + rename 原子写回，非 sed），手动修改只能存活到下次刷新。
- 运行期数据：`$MODDIR/data/battery.db`（SQLite 十表：kv/sessions/estimates/resistance/rest_points/events/samples/ccct/ica_peaks/discharge，90 天自动清理）。WAL 模式（并发读写低锁竞争），`wal_autocheckpoint=256` 页 + 每日 `wal_checkpoint(TRUNCATE)`，保证主库文件接近自包含（用户只复制 `.db` 报障时不缺最近数据）。
- `.github/workflows/release.yml` — 打 tag 后：校验标签↔version 一致 → Go 构建 → 打包两个变体 → 创建 Release → 回写 `update.json`。
- `docs/` — 论文文档库：入库文件仅限按命名规范格式化的论文 PDF（`NN-作者年份-主题-venue-分级.pdf`，全小写连字符，`NN` 按核对清单权威排序，末段为权威分级 A/B/C/D，如 `01-severson2019-nature-energy-a.pdf`）。**设计笔记、superpowers 过程文档（`docs/superpowers/` 规格与计划）、「核对报告」类中间调研文档一律只存本地工作区，禁止提交进仓库。**
- `META-INF/com/google/android/` — 标准 Magisk 刷入桩（要求 v20.4+），无需改动。

## 约定与坑

- shell 脚本是 POSIX sh（`#!/system/bin/sh`）。不要按 pwsh 或 GNU bash 习惯“修正”语法，也不要尝试本地执行验证，只能靠通读代码推演设备端行为；Go 侧改动必须本地 `go test ./...` 全绿。
- 所有面向用户的文案（描述、once 输出、WebUI）为简体中文，新增输出保持中文风格一致；数值与复合单位间有空格（`%d mAh` 为定稿格式）；百分号与量词 `次` 紧邻数字不空格（`健康 92%`、`循环 210次` 为定稿格式）。
- 双变体发布：同一源码打两个包——stable `ChargingRecordEvolution.zip` 与实验 `ChargingRecordEvolution_ML.zip`。CI 打 ML 包时改 `name=ChargingRecord Evolution ML`、description 加 `[ML实验版]` 前缀并**删除 updateJson 行**，故 ML 仅手动刷入、无应用内更新；`updateJson` 永远只指向 stable 资产。
- 两变体 id 相同互斥安装，同一次发版 version/versionCode 相同。
- 历史 id 曾为 `Charging_Record`：从旧 id 升级必须先卸载旧模块再刷入（README 安装节已注明），模块目录与数据目录随之迁移。
- 打包发布由 `.github/workflows/release.yml` 完成；不要把生成的 zip 提交进仓库。
- WebUI 取数 JSON 的顶层字段与 `cmd/batteryd/jsonout.go` 一一对应（recent 按 ts 倒序、`delta_pct` 为容量百分点、空切片输出 `[]` 非 null）；新增字段须两端同步并保持「缺失即省略」降级纪律。samples/ccct/ica_peaks/discharge 等新表全部接入 90 天清理。
- 时区：设备上 Go 运行时读不到系统时区（无 tzdata 路径），`time.Now()` 返回 UTC。面向用户的时间戳一律走 `localNow()`（getprop `persist.sys.timezone` → 文件兜底 → 诚实回落 UTC，内嵌 `time/tzdata`）；Pipeline 的时钟注入同样传 `localNow`，否则 `p.now().Format` 类日志会差 8 小时。`Unix()` 取值与时区无关，不受影响。
- 双电芯判定阈值是 **5V**（不是 4.5V）：高压单电芯截止 4.45~4.53V 常见，4.5V 会误判并让所有电压窗口翻倍、CCCT/ICA 静默全哑；双电芯串联最低约 6.8V，5V 两侧余量充足。缩放函数（`initCCCTVoltage`/`initICAVoltage`）必须无条件赋值以支持复位，否则测试间全局状态互相污染。
- 已知局限（勿当 bug 修，属有意取舍）：current 单位启发式在涓流 <10mA 时可能误判；RLS 无遗忘因子、P 矩阵长期膨胀属潜伏项；FindNode 全树兜底仅缺节点时触发；`current_now` 走带符号读取（放电为负），其余节点严格非负；节点缺失时描述/JSON/once 输出按可用字段降级，不整体失败；放电会话仅在 `status=Discharging` 拍累计，旁路供电机型（status=Full 但电池实际放电）漏记；放电隐含容量以「显示掉幅」为分母，顶部区间显示被压缩故绝对值偏高，只作观测不入估算通道。

## 发版流程

1. 改 `module.prop` 的 `version` 与 `versionCode`（同步递增）
2. 提交到 main
3. `git tag vX.Y.Z && git push origin main --tags` —— CI 自动构建打包、创建 Release（固定资产名 `ChargingRecordEvolution.zip` / `ChargingRecordEvolution_ML.zip` / `changelog.txt`）并回写 `update.json`
4. 标签必须与 `module.prop` 的 `version` 一致（如 v1.2 ↔ version=1.2），否则 CI 直接失败
5. `versionCode` 必须严格递增，**不得复用历史值**（曾踩坑：1.3.0(15)→1.2.2(15) 撞号导致部分管理器缓存旧版本收不到更新）
