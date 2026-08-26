<div align="center">

# 🔋 Charging Record

**把电池健康度写在 root 管理器模块描述里，刷入即用，零配置**

[![Release](https://img.shields.io/github/v/release/f1owkang/CRecordEvolution?style=flat-square&label=Release&color=blue)](https://github.com/f1owkang/CRecordEvolution/releases)
[![Downloads](https://img.shields.io/github/downloads/f1owkang/CRecordEvolution/total?style=flat-square&label=Downloads&color=green)](https://github.com/f1owkang/CRecordEvolution/releases)
[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev)
[![Platform](https://img.shields.io/badge/Platform-Magisk%20%7C%20KernelSU%20%7C%20APatch%20%7C%20MMRL-blue?style=flat-square)](#安装)
[![License](https://img.shields.io/github/license/f1owkang/CRecordEvolution?style=flat-square&label=License&color=orange)](LICENSE)

[安装](#安装) ｜ [工作原理](#工作原理) ｜ [日志](#日志) ｜ [常见问题](#常见问题) ｜ [致谢](#致谢)

</div>

---

## 安装

需要 Magisk v20.4+ / KernelSU / APatch 任一（仅 arm64），从 [Releases](https://github.com/f1owkang/CRecordEvolution/releases) 下载：

| 资产 | 通道 | 说明 |
| :--: | :-- | :-- |
| `Charging_Record.zip` | ✅ 稳定 | 统计算法，支持应用内更新 |
| `Charging_Record_ML.zip` | 🧪 实验 | 在线学习算法，手动刷入，不提供应用内更新 |

1. 在管理器中选择刷入下载好的 zip
2. 按提示确认：音量+ 安装 / 音量- 取消
3. 重启后模块描述即显示电池健康数据

> [!NOTE]
> 两个变体互斥，后刷的会替换先刷的。Magisk 低版本没有 Action 按钮，等下次重启刷新即可。

## 工作原理

结果来自两条路，都写在模块描述里。

**内核读数**：直接读电量计的 `charge_full` 和 `charge_full_design` 相除。很多机器的内核会自己校准这个值，它是最省事也最稳的来源。

**实测估算**：充电时每分钟记一次电流，一次充电结束后拿充进去的电荷量除以百分比涨幅，反推整块电池现在的容量。涨幅不到 20%，或者推出来的结果超出设计容量一半到一点五倍的会话直接丢弃。

两个通道在聚合策略上分叉：

| 通道 | 聚合方式 | 出结果时间 |
| :--: | :-- | :-- |
| 稳定 | 会话温度须在 15~40 ℃，指数滑动平均（新样本权重 0.3） | 首个有效会话 |
| 🧪 ML | 四分位距剔除离群会话，在线回归学习温度 / 充电倍率 / 起始电压的影响 | 前 30 次会话与稳定一致，之后逐渐体现差异 |

> [!WARNING]
> 已知的不准情况：
> 边玩边充时负载电流会被算进充入电量，读数偏高，熄屏充电最准；
> 个别厂商的电流节点单位是 mA 而不是 µA，模块靠数值大小猜单位，猜错则全错。

## 日志

运行数据存在模块目录下的 `data/battery.db`（SQLite），只保留最近 90 天，过期数据每日自动清理。有 root 终端的话可以这样翻历史：

```sh
sqlite3 /data/adb/modules/Charging_Record/data/battery.db \
  'SELECT datetime(ts, "unixepoch", "localtime"), mah FROM estimates ORDER BY ts DESC LIMIT 10;'
```

## 常见问题

<details>
<summary><b>健康度超过 100% 是不是坏了？</b></summary>

不是。出厂标称容量普遍保守，实际满充高于设计值很常见。

</details>

<details>
<summary><b>和 AccuBattery 有什么区别？</b></summary>

思路类似，都是库仑积分，但本模块以 root 直接读内核电量计，无常驻通知、无网络权限，也不依赖 Android 的电池统计接口。

</details>

<details>
<summary><b>多久能看到实测估算？</b></summary>

正常使用几天内出第一个数。ML 版要攒够 30 次有效会话才开始体现差异。

</details>

## 致谢

- 原模块：不会梦游的鱼
- 安装脚本：酷安@阿巴酱
- 维护者：[f1owkang](https://github.com/f1owkang)

喜欢这个项目的话欢迎提 PR 或点个 Star ⭐

## License

本项目以 [GPL-2.0](LICENSE) 协议开源。

---

<div align="center">

**Made with ❤ by [f1owkang](https://github.com/f1owkang)**

</div>
