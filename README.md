# Charging Record

读取系统电池数据，在 root 管理器的模块描述中实时显示电池健康度。核心逻辑为单个静态编译的 Go 二进制，刷入即用、零配置。

## 特性

- **零配置**：刷入重启后自动工作，结果直接显示在模块卡片描述里
- **双估算来源**：
  - 内核读数：直接读取电量计的 `charge_full` / `charge_full_design` / `cycle_count`
  - 实测估算：充电过程中对电流做库仑积分，用真实充进的电荷量反推电池满充容量
- 四种刷新时机：
  - 开机完成后立即计算一次（失败自动重试）
  - 充电状态变化时（插拔充电器、充满）自动重新计算
  - 每 24 小时兜底刷新一次
  - 管理器内的 **Action 按钮** 手动刷新并输出详细统计
- **WebUI 仪表盘**：KernelSU / APatch / MMRL 中可打开模块 WebUI，查看健康度大卡、六项指标与最近估算记录（60 秒自动刷新）
- 日志落盘 SQLite（`data/battery.db`），保留 90 天并自动清理过期数据

## 兼容性

- root 环境：Magisk v20.4+ / KernelSU / APatch / MMRL 任一
- 架构：仅 android/arm64
- WebUI 仅在 KernelSU / APatch / MMRL 中可用（Magisk 无 WebUI 入口，不影响其余功能）

## 安装

1. 在 [Releases](../../releases) 下载资产：
   - `Charging_Record.zip` —— 稳定版（传统统计算法），推荐大多数用户
   - `Charging_Record_ML.zip` —— 实验版（在线学习算法），差异见下方「工作原理」与「更新」
2. 在 root 管理器中选择刷入该 zip
3. 按提示**音量+ 确认安装**（音量- 取消）
4. 重启后模块描述即显示电池健康数据

两个变体请勿同时安装（后刷入者会替换前者）。

## 工作原理

### 内核读数

从 sysfs 读取电池节点（优先 `/sys/class/power_supply/battery/`，找不到则目录遍历兜底）：

| 节点 | 含义 |
| --- | --- |
| `charge_full_design` | 出厂设计容量（µAh） |
| `charge_full` | 当前实际满充容量（µAh） |
| `cycle_count` | 电池循环次数 |

### 充电会话实测（库仑积分）

守护进程每 60 秒采样一次，仅在充电状态累计电荷量：对电流取绝对值积分（µA/mA 单位自适应识别）。一个会话在充满或拔出充电器时结算，满足以下条件才计入统计：

- 电量增量 ≥ 20%
- 推算出的满充容量落在设计容量的 50%~150% 区间内

通过校验后得到本次会话的容量估计，再按所选通道聚合。

### 两种算法通道

| 通道 | 打包资产 | 算法 |
| --- | --- | --- |
| stable | `Charging_Record.zip` | 温度硬门控（会话温度须处于 15~40 ℃）+ EMA 平滑（新样本权重 0.3），首个有效会话即出结果 |
| ml（实验） | `Charging_Record_ML.zip` | 无硬温度门控，IQR 鲁棒剔除离群会话 + 在线 RLS 学习温度/倍率/起始电压特征；前 30 个会话纯基线冷启动，之后渐进混合直至 0.5 权重 |

### 局限性

- 亮屏使用时负载电流会被混入库仑积分，实测偏低；熄屏充电时最准
- 电流单位判别采用启发式（原始读数 > 10000 视为 µA），个别厂商节点可能误判
- 学习期需要数天正常充电才能收敛出稳定的实测估算
- 重启后内阻滑窗与静息电压基线需重新积累

## 日志

运行数据保存在模块目录下 `/data/adb/modules/Charging_Record/data/battery.db`（SQLite，六张表：`kv` / `sessions` / `estimates` / `resistance` / `rest_points` / `events`），超过 90 天的记录会在每日首次刷新时自动清理。

有 root 终端时可自行查询：

```sh
# 查看最近的实测估算记录
sqlite3 /data/adb/modules/Charging_Record/data/battery.db \
  'SELECT datetime(ts, "unixepoch", "localtime"), mah FROM estimates ORDER BY ts DESC LIMIT 10;'

# 手动清理 30 天前的会话记录（日常无需操作，模块每日会自动清理 90 天前的数据）
sqlite3 /data/adb/modules/Charging_Record/data/battery.db \
  'DELETE FROM sessions WHERE end_ts < strftime("%s", "now") - 30*86400;'
```

## 更新

`module.prop` 内置 `updateJson`，Magisk / KernelSU / APatch / MMRL 可直接在应用内检查并升级到最新 Release——**仅 stable 版提供此通道**。

`Charging_Record_ML.zip` 为实验通道：打包时不写入 `updateJson`，收不到应用内更新推送，需要更新时请手动下载新版刷入。

## FAQ

**为什么估算值会超过 100%？**

厂商标定的设计容量通常偏保守，电池实际满充容量高于设计值很常见，此时 `charge_full / charge_full_design` 就会大于 100%，属于正常现象，并非计算错误。

**和 AccuBattery 这类应用有什么区别？**

AccuBattery 在用户层统计电量读数估算损耗；本模块以 root 身份直接读取内核电量计，并用库仑积分实测完整充电会话，无常驻通知、无网络权限，精度取决于厂商电量计质量而非统计模型。

**多久能看到「实测估算」？**

需要积累满足条件的完整充电会话（增量 ≥20%、温度合规等），正常使用一般几天内出首个结果；ML 版还会在前 30 个会话保持纯基线，之后再逐步引入学习修正。

**为什么核心从 shell 脚本换成了 Go？**

POSIX sh 的数值运算、SQLite 访问和原子写回都极其受限且脆弱；Go 编译出单个静态 arm64 二进制，可以稳定实现 EMA / RLS 数值计算与 SQLite 存储，并且能在本机跑单元测试（`go test ./...`）保证算法正确性。

## 致谢

- 原作者：不会梦游的鱼
- 安装脚本编写感谢：酷安@阿巴酱
- 维护者：[f1owkang](https://github.com/f1owkang)

## License

见 [LICENSE](LICENSE)。
