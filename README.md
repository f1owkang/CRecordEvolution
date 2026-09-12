<div align="center">

# 🔋 ChargingRecord Evolution

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
| :-- | :--: | :-- |
| `ChargingRecordEvolution.zip` | 稳定 | 统计算法，支持应用内更新 |
| `ChargingRecordEvolution_ML.zip` | 实验 | 在线学习算法，只能手动刷入 |

刷入就是常规流程：管理器选 zip，音量+ 确认（音量- 取消），重启。重启后模块描述就是电池健康数据，之后每次充电结算完自动更新。

几个要注意的点：

- 两个变体同 id，互斥安装，后刷的会替换先刷的；
- Magisk 低版本没有 Action 按钮，看不到按钮就等重启刷新；
- 从老版本（模块 id 为 `Charging_Record`）升级的话，先卸载旧模块再刷，两边目录不通用；
- KernelSU / APatch / MMRL 用户可以打开模块自带的 WebUI，里面有估算曲线、充电记录和各特征的走势图，图上的点都能点按看具体数值。

## 工作原理

描述里的数字来自两条独立的路。

一条是内核读数：直接拿电量计的 `charge_full` 除以 `charge_full_design`。很多机器的内核会自己校准这个值，最省事也最稳。

另一条是实测估算：充电时每 15 秒记一次电流和电压（非充电期每分钟一次），一次充电结束后，用充进去的电荷量除以百分比涨幅，反推这块电池现在的实际容量。之所以不像传统库仑计那样无限积分下去，是因为那样误差会越滚越大——每次充电都重新反推一遍，等于每次都对表，历史误差攒不起来。

会话不是照单全收。涨幅不到 20%、温度超出 15~45 ℃、或者推出来明显离谱（设计容量的一半以下或 1.5 倍以上）的直接扔掉；没充到满就拔掉的会话照样参与，但权重只有充满的三分之一——这种会话推出来的容量普遍偏高，实测数据里能差出一成。

两个变体的区别在聚合这一步。稳定版就是上面的滑动平均；ML 版多加了两层：四分位距剔除离群会话，再用在线回归学温度、充电倍率、起始电压对容量的影响。前 30 次有效会话两个版本行为一致，之后 ML 版开始体现差异，并带一个不确定度 ±。

另外模块还在顺手收集几样东西，存在本地等数据攒够了做下一步：静置电压（拔掉负载后的恢复电压，要熄屏静置约 10 分钟才收敛）、恒流时长（恒流段里充过 4.2~4.3 V 这 0.1 V 窗口的耗时，随老化变短；LFP 电池满充只有 3.65 V，永远到不了这个窗口，该特征会一直显示采集中，不是故障）、增量容量主峰（只在低倍率慢充会话上算）。这几个特征目前只看趋势方向，不承诺绝对精度。

> [!WARNING]
> 已知不准的情况：
> 边玩边充时负载电流会被算进充入电量，读数偏高，熄屏充电最准；
> 个别厂商的电流节点单位是 mA 而不是 µA，模块靠数值大小猜单位，猜错则全错；
> 缺 `cycle_count` 或 `charge_full_design` 节点的机型，对应字段从描述中省略，实测估算停在「学习期」，不影响其他功能。

## 日志

运行数据存在模块目录下的 `data/battery.db`（SQLite），只保留最近 90 天，每日自动清理。有 root 终端的话可以自己翻历史：

```sh
sqlite3 /data/adb/modules/CRecordEvolution/data/battery.db \
  'SELECT datetime(ts, "unixepoch", "localtime"), mah FROM estimates ORDER BY ts DESC LIMIT 10;'
```

## 常见问题

<details>
<summary>健康度超过 100% 是不是坏了？</summary>

不是。出厂标称容量普遍保守，实际满充高于设计值很常见。

</details>

<details>
<summary>和 AccuBattery 有什么区别？</summary>

思路类似，都是库仑积分。区别在于本模块用 root 直接读内核电量计，无常驻通知、无网络权限，也不依赖 Android 的电池统计接口。

</details>

<details>
<summary>多久能看到实测估算？</summary>

正常用几天就能出第一个数。ML 版要攒够 30 次有效会话才开始体现差异。

</details>

<details>
<summary>升级模块会不会丢学习记录？</summary>

不会。刷入脚本会把旧版本的 `data/` 目录（含 `battery.db`）复制进新版本，估算记录和循环历史完整保留。

</details>

## 论文支撑

设计过程中参考过的论文（PDF 库见 [`docs/`](docs/)）：

- He & Shin, ACM e-Energy '23 —— 静置电压指纹
- Severson et al., Nature Energy 2019 及其后续 —— 容量衰减预测
- Lin et al., Energy 2022 —— 恒流充电时长（CCCT）
- Fly & Chen 2020、MDPI Energies 2024 综述 —— 增量容量分析（ICA）
- arXiv 2410.06422 —— 概率 ML 不确定性量化
- Zhang et al., ICCK 2025 —— 库仑计累积误差

> ML 版显示的 ± 是 RLS 相对不确定度指标，不是严格的统计置信区间。

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
