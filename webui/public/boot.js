// 尽早请求 edge-to-edge，避免宿主先用安全区内缩渲染一帧；
// 页面脚本末尾还会再调一次（兼容接口稍后才就绪的宿主）。
(function () {
  try {
    if (window.ksu && typeof window.ksu.enableEdgeToEdge === 'function') {
      window.ksu.enableEdgeToEdge(true);
    } else if (window.ksu && typeof window.ksu.enableInsets === 'function') {
      window.ksu.enableInsets(true);
    }
  } catch (e) {
    /* 老宿主没有该接口 */
  }
})();

// 首屏前应用主题 / 语言 / 底栏样式，避免闪烁
(function () {
  const root = document.documentElement;

  let theme = null;
  try {
    theme = localStorage.getItem('cre_theme');
  } catch (e) {}
  function systemTheme() {
    return window.matchMedia && window.matchMedia('(prefers-color-scheme: light)').matches
      ? 'light'
      : 'dark';
  }
  if (theme !== 'light' && theme !== 'dark' && theme !== 'my') theme = systemTheme();

  // 取色模式（my）：明暗从宿主 --background 的相对亮度推导，不能依赖
  // prefers-color-scheme（KSU/APatch WebView 未随管理器主题设置 dark 标志）。
  // 宿主没注入调色板（旧版本/未启用 Monet）时首屏先退回系统主题。
  function bgLuminance(css) {
    const s = String(css || '')
      .trim()
      .toLowerCase();
    let r, g, b;
    if (s.charAt(0) === '#') {
      let h = s.slice(1);
      if (h.length === 3 || h.length === 4)
        h = h
          .split('')
          .map(function (c) {
            return c + c;
          })
          .join('');
      if (h.length !== 6 && h.length !== 8) return null;
      r = parseInt(h.slice(0, 2), 16);
      g = parseInt(h.slice(2, 4), 16);
      b = parseInt(h.slice(4, 6), 16);
    } else {
      const m = s.match(/^rgba?\(\s*(\d+)\s*[, ]\s*(\d+)\s*[, ]\s*(\d+)/);
      if (!m) return null;
      r = +m[1];
      g = +m[2];
      b = +m[3];
    }
    if (isNaN(r) || isNaN(g) || isNaN(b)) return null;
    function lin(v) {
      v = v / 255;
      return v <= 0.04045 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4);
    }
    return 0.2126 * lin(r) + 0.7152 * lin(g) + 0.0722 * lin(b);
  }
  let scheme = theme;
  if (theme === 'my') {
    let palLum = null;
    try {
      // boot.js 是经典脚本且排在样式表之后，执行时宿主 CSS 已应用（或被判失败）。
      const cs = getComputedStyle(root);
      const primary = cs.getPropertyValue('--primary').trim();
      const bg = cs.getPropertyValue('--background').trim();
      if (primary && bg) palLum = bgLuminance(bg);
    } catch (e) {}
    if (palLum == null) {
      theme = systemTheme();
      scheme = theme;
    } else {
      scheme = palLum > 0.5 ? 'light' : 'dark';
    }
  }
  root.setAttribute('data-theme', theme);
  root.setAttribute('data-scheme', scheme);
  // 纯黑背景独立开关：只在深色上生效，浅色保持现有配色。
  let amoled = null;
  try {
    amoled = localStorage.getItem('cre_amoled');
  } catch (e) {}
  root.setAttribute('data-black', amoled === '1' && scheme !== 'light' ? '1' : '0');

  let bar = null;
  try {
    bar = localStorage.getItem('cre_barmode');
  } catch (e) {}
  root.setAttribute('data-bar', bar === 'classic' ? 'classic' : 'floating');

  let lang = null;
  try {
    lang = localStorage.getItem('cre_lang');
  } catch (e) {}
  if (lang !== 'zh' && lang !== 'en') {
    let list = [];
    try {
      if (navigator.languages && navigator.languages.length) {
        list = Array.prototype.slice.call(navigator.languages);
      }
      if (navigator.language) list.push(navigator.language);
    } catch (e) {}
    let i, v;
    for (i = 0; i < list.length; i++) {
      v = String(list[i] || '')
        .toLowerCase()
        .replace(/_/g, '-');
      if (!v) continue;
      if (v === 'zh' || v.indexOf('zh-') === 0) {
        lang = 'zh';
        break;
      }
      if (v === 'en' || v.indexOf('en-') === 0) {
        lang = 'en';
        break;
      }
    }
    // 探测不到语言时回退英文，与 i18n.go/安装器统一。
    if (lang !== 'zh' && lang !== 'en') lang = 'en';
  }
  root.setAttribute('data-lang', lang);
  root.lang = lang === 'zh' ? 'zh-CN' : 'en';
})();
