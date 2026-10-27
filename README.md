<div align="center">

# 🔋 ChargingRecord Evolution

**把电池健康度写在 root 管理器模块描述里，刷入即用，零配置**

[![Release](https://img.shields.io/github/v/release/f1owkang/CRecordEvolution?style=flat-square&label=Release&color=blue)](https://github.com/f1owkang/CRecordEvolution/releases)
[![Downloads](https://img.shields.io/github/downloads/f1owkang/CRecordEvolution/total?style=flat-square&label=Downloads&color=green)](https://github.com/f1owkang/CRecordEvolution/releases)
[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev)
[![Platform](https://img.shields.io/badge/Platform-Magisk%20%7C%20KernelSU%20%7C%20APatch%20%7C%20MMRL-blue?style=flat-square)](#安装)
[![License](https://img.shields.io/github/license/f1owkang/CRecordEvolution?style=flat-square&label=License&color=orange)](LICENSE)

[English](README_EN.md) | 简体中文

[界面预览](#界面预览) ｜ [安装](#安装) ｜ [工作原理](#工作原理) ｜ [日志](#日志) ｜ [常见问题](#常见问题) ｜ [致谢](#致谢)

</div>

---

## 界面预览

WebUI 三页（概览 / 历史 / 设置）一览。截图中的电池数据为**虚构的演示数据**，仅用于展示界面效果，不代表真实设备或实测结果。

<div align="center">

| 概览 | 历史 · 记录 | 历史 · 趋势 | 设置 |
| :---: | :---: | :---: | :---: |
| <a href=".github/images/overview-zh.png"><img src=".github/images/overview-zh.png" width="200" alt="概览页面"></a> | <a href=".github/images/history-zh.png"><img src=".github/images/history-zh.png" width="200" alt="历史记录"></a> | <a href=".github/images/trends-zh.png"><img src=".github/images/trends-zh.png" width="200" alt="历史趋势"></a> | <a href=".github/images/settings-zh.png"><img src=".github/images/settings-zh.png" width="200" alt="设置页面"></a> |

</div>

## 安装

需要 Magisk v20.4+ / KernelSU / APatch 任一（仅 arm64），从 [Releases](https://github.com/f1owkang/CRecordEvolution/releases) 下载：

| 资产 | 通道 | 说明 |
| :-- | :--: | :-- |
| `CRecordEvolution-*.zip` | 稳定 | 统计算法，支持应用内更新 |
| `CRecordEvolution_ML-*.zip` | 实验 | 在线学习算法，只能手动刷入 |

### 刷入流程

管理器选 zip，先选提示语言（音量+ 中文 / 音量- English，超时跟随系统），再按音量+ 确认（音量- 取消），重启。安装器会打印设备代号、Android 版本与架构，所选语言会存进模块，之后可在 WebUI 修改。重启后模块描述就是电池健康数据，每次充电结算完自动更新。

### 注意事项

- 两个变体同 id，互斥安装；
- 升级保留 `data/battery.db` 学习记录（安装器自动复制旧数据），无需手动备份；
- Magisk 低版本没有 Action 按钮，等重启刷新；
- 从旧版（模块 id 为 `Charging_Record`）升级需先卸载旧模块，两边目录不通用；
- KernelSU / APatch / MMRL 可打开模块自带的 WebUI（概览 / 历史 / 设置），底栏可在悬浮胶囊与贴底样式间切换，图表数据点可点按查看数值。

## 多语言

模块界面支持简体中文 / English，语言按以下优先级决定：安装时显式选择（写入 `data/lang`）→ WebUI 设置页切换（同样写入 `data/lang`）→ 系统语言自动探测。

安装器、模块描述、`batteryd once` 输出、Action 提示与 WebUI 共用该设置，切换后模块描述立即刷新、无需重启；未做过选择时跟随系统语言（系统语言变更后需重启 daemon 才会刷新描述）。

## 工作原理

描述里的数字来自两条独立的路：一条读内核电量计，一条由充电过程实测反推。

<div align="center">

<picture>
  <source media="(prefers-color-scheme: dark)" srcset=".github/images/how-it-works-zh-dark.png">
  <img src=".github/images/how-it-works-zh.png" width="820" alt="工作原理流程图：采样 → 四路结算（含会话门控与质量互检）→ 聚合 → 输出">
</picture>

</div>

### 为什么这样做

- **不做无限积分**：库仑计一路积下去误差只会越滚越大；每次充电重新反推，等于每次对表，历史误差攒不起来。
- **没充满就拔掉的会话不可靠**：这类会话推出来的容量普遍偏高，实测中能差出一成。
- **中段单独校准**：顶部几个百分点往往提前报满（实测 95% 以上每格只有正常格的六成电量），中间段的百分比读数更可信。
- **两个版本前期一致**：前 30 次有效会话两版行为相同，之后 ML 版才拉开差距；它的 ± 是相对不确定度指标，不是严格的统计置信区间。
- **放电差分最稳**：不经过 `current_now`，积分在芯片内完成、不受负载突变影响，是几路口径里离散最小的一路。

### 容量趋势

三种状态：攒够 21 天数据前显示进度；测不出明显变化时显示「无显著变化」；真衰减了才显示每周掉多少。健康电池长期停在「无显著变化」是正常的——短期噪声通常大于真实衰减。

### 附加观测

这几项只作观测、不影响健康度：静置电压（拔掉负载后熄屏约 10 分钟收敛）、恒流时长（恒流段穿过 4.2~4.3 V 这 0.1 V 窗口的耗时，随老化变短；LFP 电池满充只有 3.65 V，该特征会一直显示采集中，不是故障）、增量容量主峰（仅在倍率 ≤1C 的采信会话上计算，倍率越高越不锐利）。

> [!WARNING]
> 已知不准的情况：
> - 边玩边充时负载电流可能被算进充入电量，读数偏高，熄屏充电最准（满电后长时间高负载用的情况会自动识别并停计）；
> - 个别厂商的电流节点单位是 mA 而不是 µA；模块优先按 `charge_full` 的量纲判别，没有该节点时才按数值大小推断，极端情况（涓流 <10 mA）仍可能判错；
> - 缺 `cycle_count` 的机型对应字段从描述中省略，不影响其他功能；缺 `charge_full_design` 的机型（如 VIVO/iQOO）改从设备树读设计容量，读不到时才省略该字段，实测估算停在「学习期」。

## 日志

运行数据存在模块目录下的 `data/battery.db`（SQLite），只保留最近 90 天，每日自动清理。有 root 终端可以直接查历史：

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

思路类似，都是库仑积分；区别在于本模块用 root 直接读内核电量计，无常驻通知、无网络权限，也不依赖 Android 的电池统计接口。

</details>

<details>
<summary>多久能看到实测估算？</summary>

正常用几天就能出第一个数；ML 版要攒够 30 次有效会话才开始体现差异。

</details>

<details>
<summary>升级模块会不会丢学习记录？</summary>

不会。刷入脚本会把旧版本的 `data/`（含 `battery.db`）复制进新版本，估算记录和循环历史完整保留。

</details>

## 从源码构建

WebUI 源码在 `webui/`（Preact + Vite），构建产物输出到 `webroot/`（生成目录，不提交）：

```sh
pnpm install --frozen-lockfile
pnpm run verify   # 构建 + 离线扫描 + 50KB gzip 上限
pnpm run smoke    # 无头 Chrome/Edge 冒烟（可选）
```

Go daemon 的构建与完整发版流程见 [`AGENTS.md`](AGENTS.md)。

## 论文支撑

设计过程中参考过的论文（PDF 库见 [`docs/`](docs/)）：

- He & Shin, ACM e-Energy '23 —— 静置电压指纹
- Severson et al., Nature Energy 2019 及其后续 —— 容量衰减预测
- Lin et al., Energy 2022 —— 恒流充电时长（CCCT）
- Fly & Chen 2020、MDPI Energies 2024 综述 —— 增量容量分析（ICA）
- arXiv 2410.06422 —— 概率 ML 不确定性量化
- Zhang et al., ICCK 2025 —— 库仑计累积误差

采用程度不一：Lin 2022（CCCT）与 He & Shin（静置电压）是直接采用的方法原型，参数按本机数据重新定标；其余是方法动机与背景阅读。核心容量反推是通用工程做法，不依赖上述论文。

> ML 版显示的 ± 是 RLS 相对不确定度指标，不是严格的统计置信区间。

## 贡献者

ChargingRecord Evolution 生态由以下贡献者共同打造（排名不分先后）：

<div align="center">

<a href="https://github.com/f1owkang"><img src="https://github.com/f1owkang.png" width="50" height="50" alt="f1owkang"></a>
<a href="https://github.com/hcnk3HrzQe"><img src="https://github.com/hcnk3HrzQe.png" width="50" height="50" alt="hcnk3HrzQe"></a>

</div>

## 致谢

- 设计思路：不会梦游的鱼
- 安装脚本：酷安@阿巴酱

喜欢这个项目的话欢迎提 PR 或点个 Star ⭐

## License

本项目以 [GPL-2.0](LICENSE) 协议开源。

---

<div align="center">

**Made with ❤ by [f1owkang](https://github.com/f1owkang)**

</div>
