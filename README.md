# charging_record

Magisk 模块，通过读取系统容量估算电池健康度，结果直接显示在模块管理器的模块描述里。

## 特性

- **零配置、无界面**：刷入即用，数据直接显示在 Magisk / KernelSU / APatch / MMRL 管理器的模块卡片上
- 自动刷新时机：
  - 开机完成后立即计算一次
  - 充电状态变化时（插拔充电器、充满）自动重新计算
  - 每 24 小时兜底刷新一次
  - 管理器内的 **Action 按钮** 手动刷新（Magisk v28+ / KernelSU / APatch 支持）
- 每次计算追加记录到 `/sdcard/Documents/电池健康度.log`，可长期回溯
- 支持应用内检查更新

## 工作原理

从 sysfs 读取电池节点（优先 `/sys/class/power_supply/battery/`，找不到则全盘搜索）：

| 节点 | 含义 |
| --- | --- |
| `charge_full_design` | 出厂设计容量（µAh） |
| `charge_full` | 当前实际满充容量（µAh） |
| `cycle_count` | 电池循环次数 |

估算公式：`剩余容量百分比 ≈ charge_full / charge_full_design × 100%`。

> 不同厂商、不同系统的电量计策略不同，估算精度存在差异，结果仅供参考。

## 安装

要求：Magisk v20.4+ / KernelSU / APatch 任一 root 环境。

1. 下载 Release 中的 `Charging_Record.zip`
2. 在 root 管理器中选择刷入该 zip
3. 按提示**音量+ 确认安装**（音量- 取消）
4. 重启后模块描述即显示电池健康数据

## 日志

每次刷新都会追加到 `/sdcard/Documents/电池健康度.log`（内部路径 `/data/media/0/Documents/`）。

## 更新

`module.prop` 内置 `updateJson`，Magisk / KernelSU / APatch / MMRL 管理器可直接在应用内检查并升级到最新 Release。

## 发版（维护者）

1. 修改 `module.prop` 的 `version` 与 `versionCode`（两者同步递增）
2. 提交到 `main`
3. 打同名标签并推送：`git tag v<版本> && git push origin main --tags`

CI（`.github/workflows/release.yml`）将自动：校验标签与版本一致 → 打包 zip → 依据两次标签间的提交记录生成 changelog → 创建 GitHub Release（固定资产名 `Charging_Record.zip`、`changelog.txt`）→ 把 `update.json` 回写到 `main`。

## 致谢

- 原作者：不会梦游的鱼 & Kslpix
- 安装脚本编写感谢：酷安@阿巴酱

## License

见 [LICENSE](LICENSE)。
