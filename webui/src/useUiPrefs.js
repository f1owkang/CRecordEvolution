import { useEffect, useState } from 'preact/hooks';
import {
  BAR_KEY,
  LANG_FILE,
  LANG_KEY,
  THEME_KEY,
  applyThemeDocument,
  hasExec,
  initialLang,
  run,
  storedBarMode,
  storedThemeMode,
  syncModuleLang,
} from './core.js';

const HAS_HOST_EXEC = hasExec();

// 语言 / 主题 / 底栏三个 UI 偏好：状态、对应 DOM 属性的落盘、系统配色监听与
// localStorage 写入。语言切换还要原子写 data/lang 并让 batteryd once 刷新模块描述。
function useUiPrefs() {
  const [lang, setLang] = useState(initialLang());
  const [themeMode, setThemeMode] = useState(storedThemeMode());
  const [barMode, setBarMode] = useState(storedBarMode());

  // 语言：同步 <html data-lang lang>，供 boot.js 与日期/单位格式化复用。
  useEffect(
    function () {
      document.documentElement.setAttribute('data-lang', lang);
      document.documentElement.lang = lang === 'zh' ? 'zh-CN' : 'en';
    },
    [lang]
  );

  // 主题：写 data-theme + meta[theme-color]；auto 时跟随系统配色变化。
  useEffect(
    function () {
      applyThemeDocument(themeMode);
      if (themeMode !== 'auto' || !window.matchMedia) return;
      const mq = window.matchMedia('(prefers-color-scheme: light)');
      function onChange() {
        applyThemeDocument('auto');
      }
      if (mq.addEventListener) mq.addEventListener('change', onChange);
      else if (mq.addListener) mq.addListener(onChange);
      return function () {
        if (mq.removeEventListener) mq.removeEventListener('change', onChange);
        else if (mq.removeListener) mq.removeListener(onChange);
      };
    },
    [themeMode]
  );

  // 底栏：悬浮胶囊 / 传统贴底。
  useEffect(
    function () {
      document.documentElement.setAttribute('data-bar', barMode);
    },
    [barMode]
  );

  // 挂载时读安装器写入的 data/lang，优先级高于 localStorage 与 navigator。
  useEffect(function () {
    if (!HAS_HOST_EXEC) return;
    run('cat ' + LANG_FILE).then(
      function (r) {
        if (!r || Number(r.errno) !== 0) return;
        const v = String(r.stdout || '')
          .trim()
          .toLowerCase();
        const parsed = v.indexOf('zh') === 0 ? 'zh' : v.indexOf('en') === 0 ? 'en' : '';
        if (!parsed) return;
        setLang(function (prev) {
          if (prev === parsed) return prev;
          try {
            localStorage.setItem(LANG_KEY, parsed);
          } catch (e) {}
          return parsed;
        });
      },
      function () {}
    );
  }, []);

  function changeLang(next) {
    if (next !== 'zh' && next !== 'en') return;
    if (next === lang) return;
    setLang(next);
    try {
      localStorage.setItem(LANG_KEY, next);
    } catch (e) {}
    syncModuleLang(next);
  }

  function changeTheme(next) {
    if (next !== 'auto' && next !== 'light' && next !== 'dark') return;
    setThemeMode(next);
    try {
      localStorage.setItem(THEME_KEY, next);
    } catch (e) {}
  }

  function changeBar(next) {
    if (next !== 'floating' && next !== 'classic') return;
    setBarMode(next);
    try {
      localStorage.setItem(BAR_KEY, next);
    } catch (e) {}
  }

  return {
    lang,
    themeMode,
    barMode,
    setLang: changeLang,
    setThemeMode: changeTheme,
    setBarMode: changeBar,
  };
}

export { useUiPrefs };
