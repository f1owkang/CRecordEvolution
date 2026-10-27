import { defineConfig } from 'vite';
import preact from '@preact/preset-vite';

// 源码在 webui/，构建产物写到 webroot/（模块 zip 里 WebView 实际加载的目录）。
// webroot/ 是生成物，不提交；CI 与本地都先 build 再打包/冒烟。
export default defineConfig({
  root: 'webui',
  base: './',
  publicDir: 'public',
  plugins: [
    // /internal/insets.css 是 root manager 的虚拟资源，构建期不能去解析它。
    // 用占位符骗过 Vite 的 HTML URL 解析，再在产物里换回真正路径。
    {
      name: 'cre-insets-css',
      transformIndexHtml: {
        order: 'post',
        handler(html) {
          return (
            html
              .replace('__INSETS_CSS__', '/internal/insets.css')
              // 部分管理器的 WebView 对 custom scheme 下的 crossorigin 更敏感；
              // 产物同源加载，去掉 Vite 默认的 crossorigin 属性更稳。
              .replace(/\s+crossorigin(?:="")?/g, '')
          );
        },
      },
    },
    preact(),
  ],
  build: {
    outDir: '../webroot',
    emptyOutDir: true,
    target: 'es2018',
    cssCodeSplit: false,
    assetsInlineLimit: 0,
    sourcemap: false,
    reportCompressedSize: false,
    // 单入口、无动态 import：不需要 Vite 的 modulepreload polyfill。
    // 该 polyfill 会注入 fetch()，既违反“离线/无远程请求”的产物约束，
    // 也会让 CI 的离线扫描失败。
    modulePreload: { polyfill: false },
    rollupOptions: {
      output: {
        entryFileNames: 'assets/app-[hash].js',
        chunkFileNames: 'assets/[name]-[hash].js',
        assetFileNames: 'assets/[name]-[hash][extname]',
      },
    },
  },
});
