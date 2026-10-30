import { translate } from './i18n.js';

const CMD_JSON = '/data/adb/modules/CRecordEvolution/bin/batteryd json';
const CMD_HISTORY = '/data/adb/modules/CRecordEvolution/bin/batteryd history';
const CMD_ONCE = '/data/adb/modules/CRecordEvolution/bin/batteryd once';
const CMD_DIAG = '/data/adb/modules/CRecordEvolution/bin/batteryd diag';
const LANG_FILE = '/data/adb/modules/CRecordEvolution/data/lang';
const THEME_KEY = 'cre_theme';
const LANG_KEY = 'cre_lang';
const BAR_KEY = 'cre_barmode';
const HIST_MODE_KEY = 'cre_hist_mode';
const HIST_DAYS_KEY = 'cre_hist_days';
const HIST_PAGE_LIMIT = 20;

const POLL_BASE = 60000,
  POLL_MAX = 300000,
  TIMEOUT_MS = 15000;
const CHART_W = 320,
  CHART_H = 110;
const MONTHS_EN = [
  'Jan',
  'Feb',
  'Mar',
  'Apr',
  'May',
  'Jun',
  'Jul',
  'Aug',
  'Sep',
  'Oct',
  'Nov',
  'Dec',
];
const CHANNEL_KEY = { stable: 'channel_stable', ml: 'channel_ml' };
const REASON_KEY = {
  temp_out_of_range: 'reason_temp',
  delta_lt_20: 'reason_delta',
  out_of_window: 'reason_window',
  outlier: 'reason_outlier',
};

function pad(n) {
  n = String(n);
  return n.length < 2 ? '0' + n : n;
}

function fmtTs(lang, ts) {
  const d = new Date(Number(ts) * 1000);
  if (isNaN(d.getTime())) return '--';
  if (lang === 'en') {
    return (
      MONTHS_EN[d.getMonth()] +
      ' ' +
      d.getDate() +
      ' ' +
      pad(d.getHours()) +
      ':' +
      pad(d.getMinutes())
    );
  }
  return (
    pad(d.getMonth() + 1) +
    '-' +
    pad(d.getDate()) +
    ' ' +
    pad(d.getHours()) +
    ':' +
    pad(d.getMinutes())
  );
}

function fmtRange(lang, startTs, endTs) {
  if (!startTs) return '--';
  if (!endTs) return fmtTs(lang, startTs);
  const a = new Date(Number(startTs) * 1000),
    b = new Date(Number(endTs) * 1000);
  if (isNaN(a.getTime()) || isNaN(b.getTime())) return '--';
  const sameDay =
    a.getFullYear() === b.getFullYear() &&
    a.getMonth() === b.getMonth() &&
    a.getDate() === b.getDate();
  if (lang === 'en') {
    const head =
      MONTHS_EN[a.getMonth()] +
      ' ' +
      a.getDate() +
      ' ' +
      pad(a.getHours()) +
      ':' +
      pad(a.getMinutes());
    const tail = sameDay
      ? pad(b.getHours()) + ':' + pad(b.getMinutes())
      : MONTHS_EN[b.getMonth()] +
        ' ' +
        b.getDate() +
        ' ' +
        pad(b.getHours()) +
        ':' +
        pad(b.getMinutes());
    return head + '–' + tail;
  }
  const zhHead =
    pad(a.getMonth() + 1) +
    '-' +
    pad(a.getDate()) +
    ' ' +
    pad(a.getHours()) +
    ':' +
    pad(a.getMinutes());
  const zhTail = sameDay
    ? pad(b.getHours()) + ':' + pad(b.getMinutes())
    : pad(b.getMonth() + 1) +
      '-' +
      pad(b.getDate()) +
      ' ' +
      pad(b.getHours()) +
      ':' +
      pad(b.getMinutes());
  return zhHead + '~' + zhTail;
}

function fmtInt(v) {
  return v == null ? '—' : String(v);
}
function fmtFix(v, n) {
  return v == null ? '—' : Number(v).toFixed(n);
}

function dayStart(ts) {
  const d = new Date(Number(ts) * 1000);
  if (isNaN(d.getTime())) return 0;
  return new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
}

function fmtClock(ts) {
  const d = new Date(Number(ts) * 1000);
  if (isNaN(d.getTime())) return '--';
  return pad(d.getHours()) + ':' + pad(d.getMinutes());
}

function fmtDayLabel(lang, ts) {
  const d = new Date(Number(ts) * 1000);
  if (isNaN(d.getTime())) return '--';
  const today = dayStart(Date.now() / 1000);
  const day = dayStart(ts);
  const diff = Math.round((today - day) / 86400000);
  if (diff === 0) return translate(lang, 'hist_today');
  if (diff === 1) return translate(lang, 'hist_yesterday');
  const now = new Date();
  if (lang === 'en') {
    return (
      MONTHS_EN[d.getMonth()] +
      ' ' +
      d.getDate() +
      (d.getFullYear() !== now.getFullYear() ? ', ' + d.getFullYear() : '')
    );
  }
  return (
    (d.getFullYear() !== now.getFullYear() ? d.getFullYear() + '年' : '') +
    (d.getMonth() + 1) +
    '月' +
    d.getDate() +
    '日'
  );
}

function fmtShortDate(lang, ts) {
  const d = new Date(Number(ts) * 1000);
  if (isNaN(d.getTime())) return '--';
  return lang === 'en'
    ? MONTHS_EN[d.getMonth()] + ' ' + d.getDate()
    : d.getMonth() + 1 + '/' + d.getDate();
}

function normalize(a, b, c) {
  if (a && typeof a === 'object' && ('errno' in a || 'stdout' in a)) return a;
  return {
    errno: Number(a) || 0,
    stdout: b == null ? '' : String(b),
    stderr: c == null ? '' : String(c),
  };
}

function pickHost() {
  if (typeof ksu !== 'undefined' && ksu && typeof ksu.exec === 'function') return ksu;
  if (window.APatch && typeof window.APatch.exec === 'function') return window.APatch;
  if (window.apatch && typeof window.apatch.exec === 'function') return window.apatch;
  return null;
}

function hasExec() {
  return pickHost() != null;
}

function requestEdgeToEdge() {
  // 不依赖 exec 是否存在：即使宿主只暴露了 edge-to-edge 接口也直接调用。
  try {
    if (typeof ksu !== 'undefined' && ksu) {
      if (typeof ksu.enableEdgeToEdge === 'function') {
        ksu.enableEdgeToEdge(true);
        return;
      }
      if (typeof ksu.enableInsets === 'function') {
        ksu.enableInsets(true);
        return;
      }
    }
  } catch (e) {
    /* 老宿主没有该接口 */
  }
  const host = pickHost();
  if (!host) return;
  try {
    if (typeof host.enableEdgeToEdge === 'function') {
      host.enableEdgeToEdge(true);
      return;
    }
    if (typeof host.enableInsets === 'function') host.enableInsets(true);
  } catch (e) {
    /* 老宿主没有该接口 */
  }
}

function cbNameExec(host, cmd) {
  return new Promise(function (resolve) {
    const name = '__cr_cb_' + Date.now() + '_' + Math.floor(Math.random() * 1e6);
    let done = false;
    window[name] = function (errno, stdout, stderr) {
      if (done) return;
      done = true;
      try {
        delete window[name];
      } catch (e) {
        window[name] = undefined;
      }
      resolve(normalize(errno, stdout, stderr));
    };
    try {
      host.exec(cmd, '{}', name);
    } catch (e) {
      if (!done) {
        done = true;
        resolve({ errno: -1, stdout: '', stderr: String(e) });
      }
    }
  });
}

function promiseExec(host, cmd) {
  return new Promise(function (resolve) {
    let p;
    try {
      p = host.exec(cmd);
    } catch (e) {
      resolve(null);
      return;
    }
    if (p && typeof p.then === 'function') {
      p.then(
        function (r) {
          resolve(r && typeof r === 'object' ? normalize(r) : null);
        },
        function () {
          resolve(null);
        }
      );
    } else {
      resolve(null);
    }
  });
}

function directCbExec(host, cmd) {
  return new Promise(function (resolve) {
    try {
      host.exec(cmd, function (a, b, c) {
        resolve(normalize(a, b, c));
      });
    } catch (e) {
      resolve(null);
    }
  });
}

function run(cmd) {
  const host = pickHost();
  if (!host) return Promise.resolve({ errno: -1, nohost: true, stdout: '', stderr: '' });
  const kinds = [cbNameExec, promiseExec, directCbExec];
  return new Promise(function (resolve) {
    let settled = false;
    const timer = setTimeout(function () {
      if (!settled) {
        settled = true;
        resolve({ errno: -1, stdout: '', stderr: 'timeout' });
      }
    }, TIMEOUT_MS);
    function attempt(i) {
      if (settled) return;
      kinds[i](host, cmd).then(function (r) {
        if (settled) return;
        if (r && Number(r.errno) !== -1) {
          settled = true;
          clearTimeout(timer);
          resolve(r);
          return;
        }
        if (i + 1 < kinds.length) {
          setTimeout(function () {
            attempt(i + 1);
          }, 350);
          return;
        }
        settled = true;
        clearTimeout(timer);
        resolve({ errno: -1, stdout: '', stderr: '' });
      });
    }
    attempt(0);
  });
}

let langSyncBusy = false;
let langPending = null;
function syncModuleLang(lang) {
  // 连续快速切换时排队最后一次，避免旧写请求先完成覆盖新选择
  if (langSyncBusy) {
    langPending = lang;
    return;
  }
  langSyncBusy = true;
  const cmd =
    "printf '%s\\n' " + lang + ' > ' + LANG_FILE + '.tmp && mv ' + LANG_FILE + '.tmp ' + LANG_FILE;
  run(cmd).then(
    function () {
      langSyncBusy = false;
      if (langPending) {
        const next = langPending;
        langPending = null;
        syncModuleLang(next);
        return;
      }
      run(CMD_ONCE);
    },
    function () {
      langSyncBusy = false;
      langPending = null;
    }
  );
}

function contentFP(doc) {
  return JSON.stringify(doc, function (k, v) {
    // updated 只是采样时间，内容没变时不重绘；温度变化要重绘，但按 0.1℃ 粒度
    // 纳入指纹，避免浮点尾数抖动让整棵树每秒重建。
    if (k === 'updated') return undefined;
    if (k === 'temp_c') {
      const n = Number(v);
      return isFinite(n) ? Math.round(n * 10) / 10 : v;
    }
    return v;
  });
}

function storedThemeMode() {
  let s = null;
  try {
    s = localStorage.getItem(THEME_KEY);
  } catch (e) {}
  return s === 'light' || s === 'dark' ? s : 'auto';
}

function systemTheme() {
  try {
    if (window.matchMedia && window.matchMedia('(prefers-color-scheme: light)').matches)
      return 'light';
  } catch (e) {}
  return 'dark';
}

function resolvedTheme(mode) {
  return mode === 'auto' ? systemTheme() : mode;
}

function applyThemeDocument(mode) {
  const theme = resolvedTheme(mode);
  document.documentElement.setAttribute('data-theme', theme);
  const meta = document.querySelector('meta[name="theme-color"]');
  if (meta) meta.setAttribute('content', theme === 'light' ? '#f6f8fa' : '#0b0e14');
  try {
    const api = window.$CRecordEvolution;
    if (api && typeof api.setLightStatusBars === 'function') {
      api.setLightStatusBars(theme === 'light');
    }
  } catch (e) {
    /* 非 MMRL 宿主 */
  }
}

function storedBarMode() {
  let s = null;
  try {
    s = localStorage.getItem(BAR_KEY);
  } catch (e) {}
  return s === 'classic' ? 'classic' : 'floating';
}

function storedHistMode() {
  let s = null;
  try {
    s = localStorage.getItem(HIST_MODE_KEY);
  } catch (e) {}
  return s === 'trends' ? 'trends' : 'records';
}

function storedHistDays() {
  let s = null;
  try {
    s = Number(localStorage.getItem(HIST_DAYS_KEY));
  } catch (e) {}
  return s === 7 || s === 30 || s === 90 ? s : 30;
}

function initialLang() {
  return document.documentElement.getAttribute('data-lang') === 'en' ? 'en' : 'zh';
}

// 复制到剪贴板：优先 Clipboard API，失败回退隐藏 textarea + execCommand。
// 两者都失败时返回 false，由调用方给出“长按手动复制”的降级 UI。
function copyText(text) {
  if (navigator.clipboard && typeof navigator.clipboard.writeText === 'function') {
    return navigator.clipboard.writeText(text).then(
      function () {
        return true;
      },
      function () {
        return legacyCopy(text);
      }
    );
  }
  return Promise.resolve(legacyCopy(text));
}

function legacyCopy(text) {
  try {
    const ta = document.createElement('textarea');
    ta.value = text;
    ta.setAttribute('readonly', '');
    ta.style.position = 'fixed';
    ta.style.top = '0';
    ta.style.left = '0';
    ta.style.opacity = '0';
    document.body.appendChild(ta);
    ta.select();
    ta.setSelectionRange(0, ta.value.length);
    const ok = document.execCommand('copy');
    document.body.removeChild(ta);
    return ok;
  } catch (e) {
    return false;
  }
}

export {
  CMD_JSON,
  CMD_HISTORY,
  CMD_ONCE,
  CMD_DIAG,
  LANG_FILE,
  THEME_KEY,
  LANG_KEY,
  BAR_KEY,
  HIST_MODE_KEY,
  HIST_DAYS_KEY,
  HIST_PAGE_LIMIT,
  POLL_BASE,
  POLL_MAX,
  CHART_W,
  CHART_H,
  CHANNEL_KEY,
  REASON_KEY,
  fmtTs,
  fmtRange,
  fmtInt,
  fmtFix,
  dayStart,
  fmtClock,
  fmtDayLabel,
  fmtShortDate,
  contentFP,
  storedThemeMode,
  applyThemeDocument,
  storedBarMode,
  storedHistMode,
  storedHistDays,
  initialLang,
  hasExec,
  requestEdgeToEdge,
  run,
  copyText,
  syncModuleLang,
};
