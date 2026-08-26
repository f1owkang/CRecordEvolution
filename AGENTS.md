# AGENTS.md

Magisk 模块「Charging_Record」：读取 sysfs 估算电池健康度。**所有 shell 脚本都在 Android 设备上由 Magisk 执行，本机（Windows）无法运行或验证**——没有构建、测试、lint 命令。

## 文件职责

- `module.prop` — 模块元数据。注意：`description=` 行每次开机会被 `service.sh` 用 sed 删除后重写为实时电池健康数据（Magisk 应用中显示的描述就是它）。手动修改 description 只能存活到下次重启。
- `service.sh` — 开机延迟服务（轮询等 `sys.boot_completed=1`）。从 `/sys/devices` 下 find 出 `charge_full_design` / `charge_full` / `cycle_count`，计算容量与剩余百分比，改写 `module.prop` 的 description，并追加日志到 `/data/media/0/Documents/电池健康度.log`（即用户可见的 `/sdcard/Documents/`）。
- `customize.sh` — 刷入时的安装交互脚本：打印设备信息后用音量键确认（音量+ 安装 / 音量- abort）。
- `META-INF/com/google/android/` — 标准 Magisk 刷入桩（要求 v20.4+），无需改动。

## 约定与坑

- 脚本是 POSIX sh（`#!/system/bin/sh` / `#!/sbin/sh`）。不要按 pwsh 或 GNU bash 习惯“修正”语法，也不要尝试本地执行验证，只能靠通读代码推演设备端行为。
- 所有面向用户的文案（日志、description）为简体中文，新增输出保持中文风格一致。
- 打包发布 = 将仓库根目录内容压成 zip：`module.prop` 与 `META-INF/` 必须位于 zip 根层。打包由 `.github/workflows/release.yml` 完成；不要把生成的 zip 提交进仓库。
- 发版时同步递增 `module.prop` 的 `version` 与 `versionCode`。

## 发版流程

1. 改 `module.prop` 的 `version` 与 `versionCode`（同步递增）
2. 提交到 main
3. `git tag vX.Y && git push origin main --tags` —— CI 自动打包、创建 Release（固定资产名 `Charging_Record.zip` / `changelog.txt`）并回写 `update.json`
4. 标签必须与 `module.prop` 的 `version` 一致（如 v1.1 ↔ version=1.1），否则 CI 直接失败
