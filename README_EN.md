<div align="center">

# 🔋 ChargingRecord Evolution

**Battery health written right into your root manager's module description — flash and go, zero configuration**

[![Release](https://img.shields.io/github/v/release/f1owkang/CRecordEvolution?style=flat-square&label=Release&color=blue)](https://github.com/f1owkang/CRecordEvolution/releases)
[![Downloads](https://img.shields.io/github/downloads/f1owkang/CRecordEvolution/total?style=flat-square&label=Downloads&color=green)](https://github.com/f1owkang/CRecordEvolution/releases)
[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev)
[![Platform](https://img.shields.io/badge/Platform-Magisk%20%7C%20KernelSU%20%7C%20APatch%20%7C%20MMRL-blue?style=flat-square)](#installation)
[![License](https://img.shields.io/github/license/f1owkang/CRecordEvolution?style=flat-square&label=License&color=orange)](LICENSE)

English | [简体中文](README.md)

[Installation](#installation) | [How it works](#how-it-works) | [Logs](#logs) | [FAQ](#faq) | [Acknowledgements](#acknowledgements)

</div>

---

## Installation

Requires Magisk v20.4+, KernelSU, or APatch (arm64 only). Download from [Releases](https://github.com/f1owkang/CRecordEvolution/releases):

| Asset | Channel | Notes |
| :-- | :--: | :-- |
| `CRecordEvolution-*.zip` | Stable | Statistical algorithm, supports in-app updates |
| `CRecordEvolution_ML-*.zip` | Experimental | Online-learning algorithm, manual flash only |

Flashing is the usual routine: select the zip in your manager, choose the installer language (Volume Up = Chinese, Volume Down = English, timeout = follow system), confirm with Volume Up (Volume Down cancels), reboot. The installer prints the device codename, Android version, build number and ABI, and explains what the module does, the WebUI entry point, and that upgrades keep the learning history; an explicit language choice is stored in the module and also applies to the module description and the WebUI (and can be changed later in the WebUI). After the reboot the module description becomes your battery health data, and it refreshes automatically after each charge session settles.

A few things worth noting:

- Both variants share the same id and are mutually exclusive — flashing one replaces the other;
- Upgrades keep the `data/battery.db` learning history (the installer copies the old data into the new module directory automatically), so no manual backup is needed;
- Older Magisk versions have no Action button; if you don't see one, wait for the reboot to refresh;
- Upgrading from an old version (module id `Charging_Record`)? Uninstall the old module first, then flash — the two directory layouts are not interchangeable;
- KernelSU / APatch / MMRL users can open the bundled WebUI: three pages (Overview / History / Settings) with bottom navigation (floating pill or classic edge-to-edge bar). Overview shows the health and capacity summary; History has a date-grouped timeline, 7/30/90-day range and charge/discharge filters on one row, paginated older records, large capacity and charge-session charts, and discharge/resting/CCCT/ICA trend small multiples; Settings lets you switch theme/language and refresh manually; every point on a chart can be tapped to see its exact value.

## Languages

The module UI supports Simplified Chinese and English. The language is chosen in this order: an explicit choice at install time (stored in the module's `data/lang`) → a manual switch on the WebUI Settings page (also stored in `data/lang`) → automatic detection of the system language. The installer, the module description, `batteryd once` output, Action button messages and the WebUI all share this setting; the language selector on the WebUI Settings page switches at any time and immediately refreshes the module description, with no reboot needed. With no stored choice the module follows the system language (after a system language change, the module description refreshes only after the daemon restarts).

## How it works

The numbers in the description come from two independent paths.

One is the kernel reading: take the fuel gauge's `charge_full` and divide it by `charge_full_design`. On many devices the kernel calibrates this value itself, which makes it both the easiest and the most stable option.

The other is a measured estimate: while charging, current and voltage are sampled every 15 seconds (once a minute the rest of the time), and when a charge session ends the charge that went in is divided by the percentage gain to derive the battery's current actual capacity. The module doesn't integrate indefinitely the way a conventional coulomb counter does: that error keeps compounding, whereas re-deriving the capacity on every charge is like re-syncing a clock each time, so historical error never accumulates.

Both readings include their corresponding capacity in mAh in the description, followed by the current battery temperature; when no measured value is available, full-charge/design capacities bridge the gap.

Not every session is accepted. Sessions with less than a 20% gain, temperatures outside 15–45 °C, or obviously implausible results (below half or above 1.5× the design capacity) are discarded outright; sessions unplugged before reaching full still count, but at one third of the weight of a full charge — their derived capacity tends to run high, by as much as 10% in measured data.

Not reaching full is no obstacle either. The module also takes the middle stretch of each charge (30%–90%) and works it out separately: percentage readings are the most honest there, while the top few percentage steps tend to report full early (in measurements, above 95% each step holds only about 60% of the charge of a normal step — it rises fast while charging and falls slowly while discharging, both for the same reason). This path produces a number after just a few charges, is calibrated at most once a day, and is weighted very lightly: it only fine-tunes and never dominates.

The two variants differ at the aggregation step. The Stable build uses the moving average described above; ML adds two layers: interquartile-range rejection of outlier sessions, then an online regression that learns how temperature, charge rate, and starting voltage affect capacity. The two behave identically for the first 30 valid sessions; after that the ML build starts to diverge from Stable, and it reports an uncertainty figure (±).

The capacity trend on the home page has three states: it shows progress until 21 days of data have accumulated; "no significant change" once there is enough data but no clear movement can be measured; and a weekly loss figure only once real degradation sets in. A healthy battery sitting at "no significant change" for a long time is normal — short-term noise is far larger than real degradation, and capacity doesn't decline linearly either.

The module also quietly collects a few more things locally while the data builds up for the next step: resting voltage (the recovered voltage after the load is removed; it takes about 10 minutes of screen-off rest to converge), constant-current time (the time spent charging through the 0.1 V window between 4.2 V and 4.3 V during the CC phase, which shortens with age; a fully charged LFP battery only reaches 3.65 V and never enters that window, so the feature will keep showing as still collecting — not a fault), and the incremental-capacity main peak (computed from accepted sessions at ≤1C; the higher the charge rate, the less sharp the peak). Discharge is tracked too: the module reads the fuel gauge chip's own coulomb counter differentially (the integration happens inside the chip, immune to load transients), a session starts when the charger is unplugged and settles when it is plugged back in, recording how much charge was used and how many percentage points were lost. Of these, resting voltage, constant-current time, and the incremental-capacity main peak currently only indicate a trend direction, or are purely observational — none of them promises absolute accuracy, and none affects health; the discharge differential is folded into the measured estimate at low weight (it never goes through `current_now` and is the least scattered of the several capacity paths), while charging sessions remain the primary source.

> [!WARNING]
> Known inaccuracies:
> Using the phone while charging can cause load current to be counted as charge input, so readings run high; screen-off charging is the most accurate (the module detects prolonged high-load use after reaching full and stops counting it);
> Current nodes on some vendors' devices are in mA rather than µA; the module infers the current unit mainly from the scale of `charge_full` and falls back to magnitude-based inference only when that node is missing, so extreme cases (trickle below 10 mA) can still be misjudged;
> On devices without a `cycle_count` node the corresponding field is omitted from the description without affecting anything else; devices without `charge_full_design` (e.g. VIVO/iQOO) read the design capacity from the device tree instead, and only if that fails is the field omitted, leaving the measured estimate at "Learning period".

## Logs

Runtime data lives in `data/battery.db` (SQLite) under the module directory, kept for the last 90 days and pruned automatically every day. With a root terminal you can dig through the history yourself:

```sh
sqlite3 /data/adb/modules/CRecordEvolution/data/battery.db \
  'SELECT datetime(ts, "unixepoch", "localtime"), mah FROM estimates ORDER BY ts DESC LIMIT 10;'
```

## FAQ

<details>
<summary>Is a health reading above 100% a sign of something broken?</summary>

No. Factory-rated capacity is generally conservative, so the actual full-charge capacity often exceeds the design value.

</details>

<details>
<summary>How is this different from AccuBattery?</summary>

The idea is similar — both integrate charge. The difference is that this module uses root to read the kernel fuel gauge directly: no persistent notification, no network permission, and no dependency on Android's battery stats API.

</details>

<details>
<summary>How long until I see a measured estimate?</summary>

A few days of normal use gets you the first number. The ML build needs 30 valid sessions before it starts to diverge from Stable.

</details>

<details>
<summary>Will upgrading the module lose my learning history?</summary>

No. The flash script copies the old version's `data/` directory (including `battery.db`) into the new version, so estimate records and cycle history are preserved intact.

</details>

## Research

Papers consulted during design (PDF library in [`docs/`](docs/)):

- He & Shin, ACM e-Energy '23 — resting-voltage fingerprinting
- Severson et al., Nature Energy 2019 and follow-ups — capacity fade prediction
- Lin et al., Energy 2022 — constant-current charge time (CCCT)
- Fly & Chen 2020, MDPI Energies 2024 review — incremental capacity analysis (ICA)
- arXiv 2410.06422 — probabilistic ML uncertainty quantification
- Zhang et al., ICCK 2025 — coulomb-counter accumulation error

The degree of adoption varies: Lin 2022 (CCCT) and He & Shin (resting voltage) are the method prototypes adopted directly, with parameters recalibrated against the development device's data; the rest served as motivation and background reading. The core capacity back-calculation is a standard engineering practice and does not depend on the papers above.

> The ± shown by the ML build is an RLS relative-uncertainty indicator, not a strict statistical confidence interval.

## Contributors

ChargingRecord Evolution is built by the following contributors (in no particular order):

<div align="center">

<a href="https://github.com/f1owkang"><img src="https://github.com/f1owkang.png" width="50" height="50" alt="f1owkang"></a>
<a href="https://github.com/hcnk3HrzQe"><img src="https://github.com/hcnk3HrzQe.png" width="50" height="50" alt="hcnk3HrzQe"></a>

</div>

## Acknowledgements

- Design concept: 不会梦游的鱼
- Install script: 酷安@阿巴酱

If you like the project, PRs and stars are welcome ⭐

## License

This project is licensed under [GPL-2.0](LICENSE).

---

<div align="center">

**Made with ❤ by [f1owkang](https://github.com/f1owkang)**

</div>
