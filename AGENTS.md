# AGENTS.md

Magisk 模块「Charging_Record」：Go 二进制 `batteryd` 读取 sysfs 并结合充电会话库仑积分估算电池健康度，结果显示在管理器的模块描述里。业务逻辑全部在 Go（`cmd/batteryd/`）；仓库里的 `service.sh` / `action.sh` / `customize.sh` 只是 POSIX sh 引导/交互壳，在 Android 设备上由 root 管理器执行，本机无法运行验证。

## 构建

- 本机验证：`go test ./...`（Windows 可直接跑，modernc.org/sqlite 纯 Go 无 CGO）。
- 设备端构建命令（CI 同款，android/arm64 静态二进制）：
  `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o bin/batteryd ./cmd/batteryd`
- `bin/batteryd` 是 CI 打包进 zip 的产物，不要提交进仓库；本地构建残留（如 `batteryd.exe`）同样勿提交。

## 文件职责

- `cmd/batteryd/` — 全部业务逻辑：sysfs 探测与单位判别、60s 采样管道与会话结算、stable/ml 双通道 Estimator、SQLite 存储与 90 天清理、description 组装与原子写回、`daemon | once | json` 子命令分发。算法通道由构建注入 `-X main.channel=ml` 区分。
- `service.sh` — 开机延迟引导：等 `sys.boot_completed=1`（2s 轮询）后 `exec "$MODDIR/bin/batteryd" daemon`；首刷重试（10 次 × 30s）在 Go 内。
- `action.sh` — Action 按钮：执行 `bin/batteryd once` 并透传退出码，失败提示查 events 表。
- `customize.sh` — 刷入交互脚本：打印设备信息后音量键确认（音量+ 安装 / 音量- abort），解压后 `set_perm "$MODPATH/bin/batteryd" 0 0 0755`。
- `webroot/index.html` — KSU/APatch/MMRL WebUI 单文件仪表盘（内联 CSS/JS 零依赖），按序探测管理器注入的执行 API 调 `bin/batteryd json` 取数。权限由安装器接管，**不要给它加 chmod/set_perm**。
- `module.prop` — 模块元数据。`description=` 行由 batteryd 运行期改写为实时电池健康数据（临时文件 + rename 原子写回，非 sed）；手动修改只能存活到下次刷新。
- 运行期数据：`$MODDIR/data/battery.db`（SQLite 六表：kv/sessions/estimates/resistance/rest_points/events，90 天自动清理）。
- `.github/workflows/release.yml` — 打 tag 后：校验标签↔version 一致 → Go 构建 → 打包两个变体 → 创建 Release → 回写 `update.json`。
- `META-INF/com/google/android/` — 标准 Magisk 刷入桩（要求 v20.4+），无需改动。

## 约定与坑

- shell 脚本是 POSIX sh（`#!/system/bin/sh`）。不要按 pwsh 或 GNU bash 习惯“修正”语法，也不要尝试本地执行验证，只能靠通读代码推演设备端行为；Go 侧改动必须本地 `go test ./...` 全绿。
- 所有面向用户的文案（描述、once 输出、WebUI）为简体中文，新增输出保持中文风格一致；数值与单位间有空格（`%d mAh` 为定稿格式）。
- 双变体发布：同一源码打两个包——stable `Charging_Record.zip` 与实验 `Charging_Record_ML.zip`。CI 打 ML 包时改 `name=Charging Record ML`、description 加 `[ML实验版]` 前缀并**删除 updateJson 行**，故 ML 仅手动刷入、无应用内更新；`updateJson` 永远只指向 stable 资产。
- 两变体 id 相同互斥安装，同一次发版 version/versionCode 相同。
- 打包发布由 `.github/workflows/release.yml` 完成；不要把生成的 zip 提交进仓库。

## 发版流程

1. 改 `module.prop` 的 `version` 与 `versionCode`（同步递增）
2. 提交到 main
3. `git tag vX.Y && git push origin main --tags` —— CI 自动构建打包、创建 Release（固定资产名 `Charging_Record.zip` / `Charging_Record_ML.zip` / `changelog.txt`）并回写 `update.json`
4. 标签必须与 `module.prop` 的 `version` 一致（如 v1.2 ↔ version=1.2），否则 CI 直接失败
