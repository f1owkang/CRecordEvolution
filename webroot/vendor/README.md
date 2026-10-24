# WebUI 本地依赖（离线、无构建）

这些文件是随模块打包的本地副本，保证 WebUI 在无网络时也能启动，不需要构建步骤：

| 文件 | 版本 | 许可证 | 来源 |
| :-- | :-- | :-- | :-- |
| `preact.umd.js` | 10.27.2 | MIT（`LICENSE.preact.txt`） | `https://cdn.jsdelivr.net/npm/preact@10.27.2/dist/preact.umd.js` |
| `hooks.umd.js` | 10.27.2 | MIT（同 Preact） | `https://cdn.jsdelivr.net/npm/preact@10.27.2/hooks/dist/hooks.umd.js` |

## 说明

- 页面用 Preact 的 `h()` 手写 vnode（`webroot/index.html` 里的轻量 `E()` 工厂），**没有**使用 JSX 或 htm，因此不需要 Node/打包工具。
- **不要引入 htm**：htm 是 Apache-2.0，与本项目的 GPL-2.0-only 许可证不兼容。
- 升级 Preact 时同时更新两个 UMD 文件、`LICENSE.preact.txt` 和本文件的版本号；升级后在桌面浏览器或 Android WebView 里打开 WebUI 检查三页渲染与设置项。
