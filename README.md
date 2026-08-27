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
| :--: | :-- | :-- |
| `ChargingRecordEvolution.zip` | ✅ 稳定 | 统计算法，支持应用内更新 |
| `ChargingRecordEvolution_ML.zip` | 🧪 实验 | 在线学习算法，手动刷入，不提供应用内更新 |

1. 在管理器中选择刷入下载好的 zip
2. 按提示确认：音量+ 安装 / 音量- 取消
3. 重启后模块描述即显示电池健康数据

> [!NOTE]
> 两个变体互斥，后刷的会替换先刷的。Magisk 低版本没有 Action 按钮，等下次重启刷新即可。
> 从 v1.2.x 及更早版本（模块 id 为 `Charging_Record`）升级：请先在管理器中卸载旧模块，再刷入新版。

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
> 个别机型缺少 `cycle_count` 或 `charge_full_design` 节点时，对应字段会从描述中省略，实测估算保持「学习期」——不会因此影响其他功能。

**为什么每次充电都重新算？** 传统库仑计数把电流积分累下去，误差会随时间越滚越大（Zhang et al., ICCK 2025）。本模块每个充电会话都用百分比涨幅重新反推容量，相当于每次都校准起点和满容量，天然不积累误差。

**静置电压也在被收集。** 电池拔掉负载后的恢复电压和它的健康度高度相关（He & Shin, ACM e-Energy '23：实验室误差 <3%，静置约 10 分钟读数收敛）。模块正在积累这份数据，为将来的静置电压健康通道打底。

**恒流时长也在被收集。** 电池在恒流段里充满固定 0.1 V 电压窗口的时间，会随老化而缩短（Lin et al., Energy 2022）。模块把这段时长画成趋势曲线，只画趋势方向，不承诺绝对精度。

> [!NOTE]
> 恒流时长特征取三元锂典型电压档 3.9–4.0 V；磷酸铁锂（LFP）机型满充约 3.65 V，达不到该窗口，此特征会保持「采集中」——属有意取舍，非故障。

**增量容量主峰也在被追踪。** 充电曲线的增量容量主峰高度，会随材料老化而降低（MDPI Energies 2024 综述）。这一分析只在夜间低倍率慢充会话上进行（Fly & Chen 2020 结论迁移）。

## 日志

运行数据存在模块目录下的 `data/battery.db`（SQLite），只保留最近 90 天，过期数据每日自动清理。有 root 终端的话可以这样翻历史：

```sh
sqlite3 /data/adb/modules/CRecordEvolution/data/battery.db \
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

## 论文支撑

模块算法的扩展路线有论文依据，证据库见 [`docs/`](docs/)（`NN-作者年份-主题-venue-分级.pdf`，末段为权威分级）：

| 功能 | 依据论文 | 分级 |
| :--: | :-- | :--: |
| 静置电压指纹（远期 SoH 通道数据积累中） | He & Shin, *Fingerprinting Battery Health Using Relaxing Voltages*, ACM e-Energy '23 | A |
| 容量衰减趋势外推 | Severson et al., *Data-driven Prediction of Battery Cycle Life*, Nature Energy 2019 及其后续 arXiv 2312.05717 | A |
| 置信区间输出（ML 变体） | arXiv 2410.06422（概率 ML 不确定性量化）＋ RLS 预测方差恒等式 φᵀPφ | C |
| 恒流充电时长 CCCT 特征 | Lin et al., Energy 2022（SSRN 预印本核对） | A |
| 增量容量分析 ICA 主峰追踪 | MDPI Energies 2024 综述；Fly & Chen, J. Energy Storage 2020（低倍率约束） | B |
| 会话级重置设计合理性 | Zhang et al., ICCK 2025（库仑计累积误差，领域共识级辅助佐证） | D |

> 核对日期 2026-08-27，核心结论全部由 A/B 级同行评审文献支撑。
>
> ML 版置信区间基于 RLS 预测方差恒等式 φᵀPφ，未按噪声方差标定，属**相对不确定度指标**而非严格统计置信区间；请按趋势参考理解，勿按 ±N mAh 的统计含义解读。

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
