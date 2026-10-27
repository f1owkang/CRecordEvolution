// 尽早请求 edge-to-edge，避免宿主先用安全区内缩渲染一帧；
// 页面脚本末尾还会再调一次（兼容接口稍后才就绪的宿主）。
(function () {
  try {
    if (window.ksu && typeof window.ksu.enableEdgeToEdge === 'function') {
      window.ksu.enableEdgeToEdge(true);
    } else if (window.ksu && typeof window.ksu.enableInsets === 'function') {
      window.ksu.enableInsets(true);
    }
  } catch (e) { /* 老宿主没有该接口 */ }
})();

// 首屏前应用主题 / 语言 / 底栏样式，避免闪烁
(function () {
  var root = document.documentElement;

  var theme = null;
  try { theme = localStorage.getItem('cre_theme'); } catch (e) {}
  if (theme !== 'light' && theme !== 'dark') {
    theme = (window.matchMedia && window.matchMedia('(prefers-color-scheme: light)').matches) ? 'light' : 'dark';
  }
  root.setAttribute('data-theme', theme);

  var bar = null;
  try { bar = localStorage.getItem('cre_barmode'); } catch (e) {}
  root.setAttribute('data-bar', bar === 'classic' ? 'classic' : 'floating');

  var lang = null;
  try { lang = localStorage.getItem('cre_lang'); } catch (e) {}
  if (lang !== 'zh' && lang !== 'en') {
    var list = [];
    try {
      if (navigator.languages && navigator.languages.length) {
        list = Array.prototype.slice.call(navigator.languages);
      }
      if (navigator.language) list.push(navigator.language);
    } catch (e) {}
    var seen = false, i, v;
    for (i = 0; i < list.length; i++) {
      v = String(list[i] || '').toLowerCase().replace(/_/g, '-');
      if (!v) continue;
      seen = true;
      if (v === 'zh' || v.indexOf('zh-') === 0) { lang = 'zh'; break; }
      if (v === 'en' || v.indexOf('en-') === 0) { lang = 'en'; break; }
    }
    if (lang !== 'zh' && lang !== 'en') lang = seen ? 'en' : 'zh';
  }
  root.setAttribute('data-lang', lang);
  root.lang = lang === 'zh' ? 'zh-CN' : 'en';
})();
