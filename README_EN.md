<div align="center">

# 🔋 ChargingRecord Evolution

**Battery health written right into your root manager's module description — flash and go, zero configuration**

[![Release](https://img.shields.io/github/v/release/f1owkang/CRecordEvolution?style=flat-square&label=Release&color=blue)](https://github.com/f1owkang/CRecordEvolution/releases)
[![Downloads](https://img.shields.io/github/downloads/f1owkang/CRecordEvolution/total?style=flat-square&label=Downloads&color=green)](https://github.com/f1owkang/CRecordEvolution/releases)
[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev)
[![Platform](https://img.shields.io/badge/Platform-Magisk%20%7C%20KernelSU%20%7C%20APatch%20%7C%20MMRL-blue?style=flat-square)](#installation)
[![License](https://img.shields.io/github/license/f1owkang/CRecordEvolution?style=flat-square&label=License&color=orange)](LICENSE)

English | [简体中文](README.md)

[Features](#features) | [Screenshots](#screenshots) | [Installation](#installation) | [How it works](#how-it-works) | [Logs](#logs) | [FAQ](#faq) | [Project layout](#project-layout) | [Acknowledgements](#acknowledgements)

</div>

---

## Screenshots

The three WebUI pages (Overview / History / Settings) at a glance. The battery data in the screenshots is **fabricated demo data**, used only to illustrate the UI — it does not represent any real device or any measured result.

<div align="center">

| Overview | History · Records | History · Trends | Settings |
| :---: | :---: | :---: | :---: |
| <a href=".github/images/overview-en.png"><img src=".github/images/overview-en.png" width="200" alt="Overview page"></a> | <a href=".github/images/history-en.png"><img src=".github/images/history-en.png" width="200" alt="History records"></a> | <a href=".github/images/trends-en.png"><img src=".github/images/trends-en.png" width="200" alt="History trends"></a> | <a href=".github/images/settings-en.png"><img src=".github/images/settings-en.png" width="200" alt="Settings page"></a> |

</div>

## Features

- **Health and capacity at a glance**: the module description shows kernel health, full-charge and design capacities plus the measured estimate — no app needed.
- **Four capacity paths cross-checked**: kernel full charge, mid-stretch calibration, discharge differential and settled sessions each produce a value; a spread above 40% is written as a `quality_warn` event.
- **Three-page WebUI**: Overview / History / Settings; history filters by 7/30/90 days and by charge/discharge, records page through, and trend charts expand on tap.
- **Fully offline**: no network permission, no telemetry, no upload; data is written only to the local `data/battery.db`.
- **Multiple managers supported**: Magisk v20.4+ / KernelSU / APatch / MMRL, arm64 only; flashing keeps the previous learning history automatically.
- **90 days of history**: pruned daily; sampling runs every 15 s while charging and every 60 s otherwise.
- **Bilingual UI**: Simplified Chinese / English, selectable at install time and switchable any time in the WebUI.

## Installation

Requires Magisk v20.4+, KernelSU, or APatch (arm64 only). Download from [Releases](https://github.com/f1owkang/CRecordEvolution/releases):

| Asset | Channel | Notes |
| :-- | :--: | :-- |
| `CRecordEvolution-*.zip` | Stable | Statistical algorithm, supports in-app updates |
| `CRecordEvolution_ML-*.zip` | Experimental | Online-learning algorithm, manual flash only |

### Flashing

Pick the zip in your manager, choose the prompt language (Volume Up = Chinese, Volume Down = English, timeout = follow system), confirm with Volume Up (Volume Down cancels), reboot. The installer prints the device codename, Android version and ABI; the chosen language is stored in the module and can be changed later in the WebUI. After the reboot the module description becomes your battery health data and refreshes after each charge session.

### Notes

- Both variants share the same id and are mutually exclusive;
- Upgrades keep the `data/battery.db` learning history (the installer copies the old data automatically), so no manual backup is needed;
- Older Magisk versions have no Action button; wait for a reboot;
- Upgrading from the old version (module id `Charging_Record`)? Uninstall it first — the two directory layouts are not interchangeable;
- KernelSU / APatch / MMRL can open the bundled WebUI (Overview / History / Settings): the bottom bar toggles between a floating pill and the classic edge-to-edge style, and chart points can be tapped for exact values.

## Languages

The module UI supports Simplified Chinese and English, chosen by this priority: an explicit install-time choice (stored in `data/lang`) → a switch on the WebUI Settings page (also stored in `data/lang`) → automatic system-language detection.

The installer, module description, `batteryd once` output, Action messages and the WebUI share this setting; switching refreshes the module description immediately, with no reboot; with no stored choice it follows the system language (after a system language change the description refreshes only after the daemon restarts).

## How it works

The numbers in the description come from two independent paths: one reads the kernel fuel gauge, the other back-calculates from the charging process.

<div align="center">

<picture>
  <source media="(prefers-color-scheme: dark)" srcset=".github/images/how-it-works-en-dark.png">
  <img src=".github/images/how-it-works-en.png" width="820" alt="How it works: sampling → four settlement paths (with session gates and quality check) → aggregation → output">
</picture>

</div>

### Why it works this way

- **No indefinite integration**: a coulomb counter would keep accumulating error; re-deriving the capacity on every charge is like re-syncing a clock.
- **Partial sessions are unreliable**: their derived capacity tends to run high — by up to 10% in measured data.
- **The mid stretch gets its own calibration**: the top few percentage points tend to report full early (above 95% each step holds only about 60% of a normal step), so percentages in the middle are more trustworthy.
- **Both variants agree early on**: they behave identically for the first 30 valid sessions; after that ML pulls ahead, and its ± is a relative-uncertainty indicator rather than a strict confidence interval.
- **The discharge differential is the steadiest path**: it never goes through `current_now`; integration happens inside the chip and is immune to load transients, making it the least scattered of the paths.

### Capacity trend

Three states: progress until 21 days of data; "no significant change" when no clear movement can be measured; and a weekly loss only once real degradation sets in. A healthy battery staying at "no significant change" is normal — short-term noise is usually larger than real degradation.

### Extra observations

These are observations only and never affect health: resting voltage (converging after about 10 minutes of screen-off rest), constant-current time (the time spent crossing the 0.1 V window between 4.2 V and 4.3 V; it shortens with age, and a full LFP battery only reaches 3.65 V, so it keeps showing as collecting — not a fault), and the incremental-capacity main peak (computed only on accepted sessions at ≤1C; a higher rate makes the peak less sharp).

> [!WARNING]
> Known inaccuracies:
> - Using the phone while charging can count load current as charge input, so readings run high; screen-off charging is the most accurate (prolonged high-load use after reaching full is detected and stopped);
> - Current nodes on some vendors' devices are in mA rather than µA; the module infers the unit mainly from the scale of `charge_full` and falls back to magnitude-based inference only when that node is missing, so extreme cases (trickle below 10 mA) can still be misjudged;
> - Devices without a `cycle_count` node omit that field without affecting anything else; devices without `charge_full_design` (e.g. VIVO/iQOO) read the design capacity from the device tree; only if that fails is the field omitted, leaving the measured estimate at "Learning period".

## Logs

Runtime data lives in `data/battery.db` (SQLite) under the module directory, kept for the last 90 days and pruned automatically every day. With a root terminal you can query the history directly:

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

A few days of normal use gets you the first number; the ML build needs 30 valid sessions before it starts to diverge.

</details>

<details>
<summary>Will upgrading the module lose my learning history?</summary>

No. The flash script copies the old version's `data/` directory (including `battery.db`) into the new version, so estimate records and cycle history are preserved intact.

</details>

## Build from source

The WebUI source lives in `webui/` (Preact + Vite) and builds into `webroot/` (generated, not committed):

```sh
pnpm install --frozen-lockfile
pnpm run verify   # build + offline scan + 50KB gzip ceiling
pnpm run smoke    # headless Chrome/Edge smoke test (optional)
```

The Go daemon is pure Go (SQLite via `modernc.org/sqlite`, no CGO needed):

```sh
go test ./... -count=1
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o bin/batteryd ./cmd/batteryd
```

`bin/batteryd` is the native executable shipped in the module; its subcommands:

- `once` — refresh the module description once and print the current readings (this is what the manager's Action button runs);
- `json` — emit the JSON snapshot used by the Overview and Settings pages;
- `history` — emit the paged data used by the History page; options are `--days 1-90`, `--limit 1-50`, `--type all|charge|discharge` and `--before TS`.

On boot `service.sh` starts `batteryd daemon`; see [`AGENTS.md`](AGENTS.md) for the full release flow (tags, CI, both variant packages).

## Project layout

| Path | Purpose |
| :-- | :-- |
| `cmd/batteryd/` | Go daemon: sampling, session settlement, capacity estimation, SQLite, module description and the CLI subcommands |
| `webui/` | WebUI source (Preact + Vite); the build output goes to `webroot/` |
| `webroot/` | WebUI build output, packed into the module zip, not committed |
| `scripts/` | Build guards: offline scan, gzip ceiling, headless smoke test |
| `service.sh` | Waits for `sys.boot_completed=1`, then starts the daemon |
| `action.sh` | Manager Action: runs `batteryd once` and passes its exit code through |
| `customize.sh` | Flash interaction: language choice, install confirmation, keeping the old `data/`, binary permissions |
| `module.prop` / `module.yaml` / `module.json` / `update.json` | Module metadata, KernelSU listing and update info |
| `META-INF/com/google/android/` | Standard Magisk flash stub |
| `docs/` | Paper PDF library |
| `README.md` / `README_EN.md` / `AGENTS.md` | Chinese/English user docs and developer conventions |

## Research

Papers consulted during design (PDF library in [`docs/`](docs/)):

- He & Shin, ACM e-Energy '23 — resting-voltage fingerprinting
- Severson et al., Nature Energy 2019 and follow-ups — capacity fade prediction
- Lin et al., Energy 2022 — constant-current charge time (CCCT)
- Fly & Chen 2020, MDPI Energies 2024 review — incremental capacity analysis (ICA)
- arXiv 2410.06422 — probabilistic ML uncertainty quantification
- Zhang et al., ICCK 2025 — coulomb-counter accumulation error

Adoption varies: Lin 2022 (CCCT) and He & Shin (resting voltage) are the method prototypes adopted directly, with parameters recalibrated against this device's data; the rest served as motivation and background reading. The core capacity back-calculation is a standard engineering practice and does not depend on the papers above.

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

## Disclaimer

- Health, capacity and cycle figures are **estimates** derived from kernel readings and the charging process; they are not guaranteed to match factory ratings or professional test equipment.
- Flashing this module modifies your system's module directory — **proceed at your own risk**, and only after you understand how to flash and roll back with Magisk / KernelSU / APatch.
- Nothing here constitutes repair or safety advice; for swelling, abnormal heat or other battery faults, rely on the manufacturer's diagnostics.
- All data stays in the local `data/battery.db`; the module makes no network requests and never uploads or collects anything.

## License

This project is licensed under [GPL-2.0](LICENSE).

---

<div align="center">

**Made with ❤ by [f1owkang](https://github.com/f1owkang)**

</div>
