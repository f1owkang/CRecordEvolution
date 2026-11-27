# 安全策略 · Security Policy

## 支持的版本 / Supported versions

| 版本 | 支持 |
| :-- | :-- |
| 1.4.x | ✅ 接收安全报告 |
| < 1.4 | ❌ 请先升级到最新 Release |

只维护 stable 与 ML 两个通道的最新版本；更早版本的问题请先确认在最新版仍可复现。

## 报告漏洞 / Reporting a vulnerability

请**不要**在公开 issue 中披露可利用细节。优先使用 GitHub 的私密报告：

<https://github.com/f1owkang/CRecordEvolution/security/advisories/new>

（也可从仓库 **Security** 标签页 → **Report a vulnerability** 进入。）

如果无法使用私密报告，请开一个不含敏感细节的 issue，说明希望私下沟通，维护者会提供后续渠道。

报告请尽量包含：

- 受影响版本与通道（stable / ML）、root 管理器、Android 版本与机型；
- 复现步骤、影响范围，以及必要的 PoC（请勿附带真实设备标识）；
- 是否已在最新 Release 上复现。

## 范围 / Scope

本项目以 root 权限运行守护进程，并由管理器 WebView 执行固定命令，以下问题按安全漏洞处理：

- WebUI `exec` 命令注入、路径穿越或越权执行；
- 把用户输入、数据库内容或模块描述拼进 shell 导致的注入；
- 绕过离线约束的网络请求、遥测或数据上传；
- 发布产物、更新源（`update.json`）或构建流水线被篡改；
- 诊断信息泄露序列号、IMEI、MAC、账号等设备标识。

以下不属于安全漏洞：

- 容量/健康度估算偏差，或节点缺失导致的按字段降级；
- 刷机风险、root 管理器自身问题、第三方 ROM/内核行为；
- 需要已取得 root 与模块目录写权限才能实施的本地改动。

## 披露与响应 / Disclosure

维护者会尽量在 7 天内确认收到报告，修复后与报告者协调披露时间并发布新版本。请在修复发布前避免公开细节。
