import { useEffect, useRef, useState } from 'preact/hooks';
import {
  AMOLED_KEY,
  BAR_KEY,
  LANG_FILE,
  LANG_KEY,
  THEME_KEY,
  applyThemeDocument,
  hasExec,
  hostColorSupported,
  initialLang,
  run,
  storedAmoled,
  storedBarMode,
  storedThemeMode,
  syncModuleLang,
} from './core.js';
import { writePref } from './prefs.js';
import { adoptModuleLang, parseModuleLang } from './langPref.js';

const HAS_HOST_EXEC = hasExec();

// 语言 / 主题 / 底栏三个 UI 偏好：状态、对应 DOM 属性的落盘、系统配色监听与
// localStorage 写入。语言切换还要原子写 data/lang 并让 batteryd once 刷新模块描述。
function useUiPrefs() {
  const [lang, setLang] = useState(initialLang());
  const [themeMode, setThemeMode] = useState(storedThemeMode());
  const [amoled, setAmoledState] = useState(storedAmoled());
  const [barMode, setBarMode] = useState(storedBarMode());
  // 用户是否手动切过语言：挂载时 cat data/lang 的迟到响应不能覆盖用户刚选的档位。
  const langTouched = useRef(false);
  // null = 尚未探测完；true/false = 宿主是否注入 Material You 调色板。
  const [dynamicColor, setDynamicColor] = useState(null);

  // 取色能力探测：调色板由宿主在收到 /internal/colors.css 请求时生成，可能晚于首屏
  // （KSU/APatch 的主题组合在 WebView 打开后才完成）。先重试读变量，再重挂一次
  // stylesheet（加 query 仅作缓存击穿，宿主按路径拦截，不会产生网络请求）。
  useEffect(function () {
    let cancelled = false;
    let tries = 0;
    function reloadPalette(n) {
      const link = document.getElementById('cre-colors');
      if (link) link.href = '/internal/colors.css?retry=' + n;
    }
    function probe() {
      if (cancelled) return;
      if (hostColorSupported()) {
        setDynamicColor(true);
        return;
      }
      tries += 1;
      if (tries >= 8) {
        setDynamicColor(false);
        return;
      }
      if (tries === 3 || tries === 6) reloadPalette(tries);
      setTimeout(probe, 300);
    }
    probe();
    return function () {
      cancelled = true;
    };
  }, []);

  // 已存了取色偏好但当前宿主不支持（如换管理器/旧版 MMRL）：降级回 auto，
  // 避免页面停在 fallback 配色上看起来像坏掉。
  useEffect(
    function () {
      if (dynamicColor === false && themeMode === 'my') changeTheme('auto');
    },
    [dynamicColor, themeMode]
  );

  // 语言：同步 <html data-lang lang>，供 boot.js 与日期/单位格式化复用。
  useEffect(
    function () {
      document.documentElement.setAttribute('data-lang', lang);
      document.documentElement.lang = lang === 'zh' ? 'zh-CN' : 'en';
    },
    [lang]
  );

  // 主题：写 data-theme + data-black + meta[theme-color]；auto 时跟随系统配色变化。
  useEffect(
    function () {
      applyThemeDocument(themeMode, amoled);
      if (themeMode !== 'auto' || !window.matchMedia) return;
      const mq = window.matchMedia('(prefers-color-scheme: light)');
      function onChange() {
        applyThemeDocument('auto', amoled);
      }
      if (mq.addEventListener) mq.addEventListener('change', onChange);
      else if (mq.addListener) mq.addListener(onChange);
      return function () {
        if (mq.removeEventListener) mq.removeEventListener('change', onChange);
        else if (mq.removeListener) mq.removeListener(onChange);
      };
    },
    [themeMode, amoled, dynamicColor]
  );

  // 底栏：悬浮胶囊 / 传统贴底。
  useEffect(
    function () {
      document.documentElement.setAttribute('data-bar', barMode);
    },
    [barMode]
  );

  // 挂载时读安装器写入的 data/lang，优先级高于 localStorage 与 navigator。
  // 用户在挂载后手动切过语言（langTouched）时，迟到的响应必须丢弃，也不能再写 localStorage。
  useEffect(function () {
    if (!HAS_HOST_EXEC) return;
    run('cat ' + LANG_FILE).then(
      function (r) {
        const parsed = parseModuleLang(r);
        if (!adoptModuleLang(langTouched.current, parsed)) return;
        setLang(function (prev) {
          if (prev === parsed) return prev;
          writePref(LANG_KEY, parsed);
          return parsed;
        });
      },
      function () {}
    );
  }, []);

  function changeLang(next) {
    if (next !== 'zh' && next !== 'en') return;
    // 用户点过语言档就算“已表达偏好”：之后 data/lang 的迟到响应一律作废。
    langTouched.current = true;
    if (next === lang) return;
    setLang(next);
    writePref(LANG_KEY, next);
    syncModuleLang(next);
  }

  function changeTheme(next) {
    if (next !== 'auto' && next !== 'light' && next !== 'dark' && next !== 'my') return;
    if (next === 'my' && dynamicColor !== true) return;
    setThemeMode(next);
    writePref(THEME_KEY, next);
  }

  function changeAmoled(next) {
    setAmoledState(!!next);
    writePref(AMOLED_KEY, next ? '1' : '0');
  }

  function changeBar(next) {
    if (next !== 'floating' && next !== 'classic') return;
    setBarMode(next);
    writePref(BAR_KEY, next);
  }

  return {
    lang,
    themeMode,
    amoled,
    barMode,
    dynamicColor: dynamicColor === true,
    setLang: changeLang,
    setThemeMode: changeTheme,
    setAmoled: changeAmoled,
    setBarMode: changeBar,
  };
}

export { useUiPrefs };
