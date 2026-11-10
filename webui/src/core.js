import { translate } from './i18n.js';

const CMD_JSON = '/data/adb/modules/CRecordEvolution/bin/batteryd json';
const CMD_HISTORY = '/data/adb/modules/CRecordEvolution/bin/batteryd history';
const CMD_ONCE = '/data/adb/modules/CRecordEvolution/bin/batteryd once';
const CMD_DIAG = '/data/adb/modules/CRecordEvolution/bin/batteryd diag';
const LANG_FILE = '/data/adb/modules/CRecordEvolution/data/lang';
const THEME_KEY = 'cre_theme';
const AMOLED_KEY = 'cre_amoled';
const LANG_KEY = 'cre_lang';
const BAR_KEY = 'cre_barmode';
const HIST_MODE_KEY = 'cre_hist_mode';
const HIST_DAYS_KEY = 'cre_hist_days';
const HIST_PAGE_LIMIT = 20;

const POLL_BASE = 60000,
  POLL_CHARGE = 15000,
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
// 千分位整数（1,234）：只用于展示层；采样/契约里仍是原始整数。
function fmtGroup(v) {
  if (v == null) return '—';
  const n = Number(v);
  if (!isFinite(n)) return '—';
  return String(Math.round(n)).replace(/\B(?=(\d{3})+(?!\d))/g, ',');
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

// live.charging 存在或 status=Charging 视为处于充电会话：概览实时卡与 15s 轮询共用。
function chargeActive(doc) {
  const live = doc && doc.live;
  return !!(live && typeof live === 'object' && (live.charging || live.status === 'Charging'));
}

// 内容指纹：updated 是采样时间；calib.raw_current/current_ua 每次 json 都变，
// 但页面不用它们展示，必须从指纹剔除，否则去重永远失效。
function contentFP(doc) {
  if (!doc || typeof doc !== 'object') return JSON.stringify(doc);
  const out = {};
  Object.keys(doc).forEach(function (k) {
    // live/daemon/calib 由下面的展示粒度折叠替代，其余字段原样参与指纹
    if (k === 'updated' || k === 'live' || k === 'daemon' || k === 'calib') return;
    out[k] = doc[k];
  });
  if (doc.calib != null) out.calib = fpCalib(doc.calib);
  if (doc.live != null) out.live = fpLive(doc.live);
  if (doc.daemon != null) out.daemon = fpDaemon(doc.daemon);
  // 温度变化要重绘，但按 0.1℃ 粒度纳入指纹，避免浮点尾数抖动让整棵树重建。
  if (doc.temp_c != null) {
    const n = Number(doc.temp_c);
    out.temp_c = isFinite(n) ? Math.round(n * 10) / 10 : doc.temp_c;
  }
  return JSON.stringify(out);
}

function fpRound(v, step) {
  const n = Number(v);
  return isFinite(n) ? Math.round(n / step) * step : v;
}

function fpCopy(obj) {
  const out = {};
  Object.keys(obj).forEach(function (k) {
    out[k] = obj[k];
  });
  return out;
}

// 校准诊断：原始读数与换算电流每轮都不同，且不参与任何展示。
function fpCalib(calib) {
  if (!calib || typeof calib !== 'object') return calib;
  const out = {};
  Object.keys(calib).forEach(function (k) {
    if (k === 'raw_current' || k === 'current_ua') return;
    out[k] = calib[k];
  });
  return out;
}

// 实时值按卡片显示粒度取整（电流 10mA / 功率 0.1W / 电压 0.01V / 电量 1mAh /
// 会话时长 1 分钟），这样显示不变时指纹也不变；cap/status 原值，充电状态切换必然触发重绘。
function fpLive(live) {
  if (!live || typeof live !== 'object') return live;
  const out = fpCopy(live);
  if (out.current_ua != null) out.current_ua = fpRound(out.current_ua, 10000);
  if (out.power_mw != null) out.power_mw = fpRound(out.power_mw, 100);
  if (out.voltage_uv != null) out.voltage_uv = fpRound(out.voltage_uv, 10000);
  if (out.charging && typeof out.charging === 'object') {
    const c = fpCopy(out.charging);
    if (c.acc_uah != null) c.acc_uah = fpRound(c.acc_uah, 1000);
    // 平均电流不展示，按与瞬时电流相同的 10mA 粒度折叠，避免尾段漂移导致每轮重绘
    if (c.avg_current_ua != null) c.avg_current_ua = fpRound(c.avg_current_ua, 10000);
    if (c.eta_min != null) c.eta_min = Math.round(Number(c.eta_min));
    // 页面只显示「x 分钟」，secs 取整到 60s 与展示粒度一致
    if (c.secs != null) c.secs = fpRound(c.secs, 60);
    out.charging = c;
  }
  return out;
}

// daemon 只关心版本与「待重启」标记；ts/age_sec 每轮都变，不参与指纹。
function fpDaemon(daemon) {
  if (!daemon || typeof daemon !== 'object') return daemon;
  const out = {};
  if (daemon.version != null) out.version = daemon.version;
  if (daemon.stale != null) out.stale = daemon.stale;
  return out;
}

function storedThemeMode() {
  let s = null;
  try {
    s = localStorage.getItem(THEME_KEY);
  } catch (e) {}
  return s === 'light' || s === 'dark' || s === 'my' ? s : 'auto';
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

// 宿主取色能力探测：MMRL/WebUI-X 通过 /internal/colors.css 注入 M3 调色板
// （--primary/--background 等）；官方 KernelSU/APatch 不提供，探测为空即降级。
// 必须读 computed style：变量由宿主样式表在 :root 上定义，不是内联声明。
function hostColorSupported() {
  try {
    const cs = getComputedStyle(document.documentElement);
    const primary = cs.getPropertyValue('--primary').trim();
    const background = cs.getPropertyValue('--background').trim();
    return !!(primary && background);
  } catch (e) {
    return false;
  }
}

function hostPaletteBackground() {
  try {
    return getComputedStyle(document.documentElement).getPropertyValue('--background').trim();
  } catch (e) {
    return '';
  }
}

// 宿主调色板的明暗方案必须从 --background 亮度推导：KSU/APatch 的 WebView
// 没有随管理器主题设置 prefers-color-scheme，matchMedia 不可信。
function cssColorLuminance(css) {
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

function paletteScheme() {
  const lum = cssColorLuminance(hostPaletteBackground());
  return lum == null ? systemTheme() : lum > 0.5 ? 'light' : 'dark';
}

// 纯黑背景是独立开关（cre_amoled），只在解析出的深色主题上叠加：
// 这样 auto 仍跟随系统，浅色不会被压黑，也无需把主题档位扩成四档。
function storedAmoled() {
  let s = null;
  try {
    s = localStorage.getItem(AMOLED_KEY);
  } catch (e) {}
  return s === '1';
}

// theme: auto|light|dark|my。my = 宿主 Material You 取色，方案从 --background 亮度推导；
// data-theme 保留 'my' 让 CSS 映射宿主变量，data-scheme 供 color-scheme 与状态栏使用。
function applyThemeDocument(mode, amoled) {
  const scheme = mode === 'my' ? paletteScheme() : resolvedTheme(mode);
  const black = !!amoled && scheme !== 'light';
  const root = document.documentElement;
  root.setAttribute('data-theme', mode === 'my' ? 'my' : scheme);
  root.setAttribute('data-scheme', scheme);
  root.setAttribute('data-black', black ? '1' : '0');
  const meta = document.querySelector('meta[name="theme-color"]');
  if (meta) {
    // 用当前实际底色（含 Material You / AMOLED）同步状态栏，避免宿主取到旧色。
    const bodyBg = getComputedStyle(document.body).backgroundColor;
    const paletteBg = mode === 'my' ? hostPaletteBackground() : '';
    const builtin = scheme === 'light' ? '#f6f8fa' : '#0b0e14';
    meta.setAttribute('content', black ? '#000000' : paletteBg || bodyBg || builtin);
  }
  try {
    const api = window.$CRecordEvolution;
    if (api && typeof api.setLightStatusBars === 'function') {
      api.setLightStatusBars(scheme === 'light');
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
  AMOLED_KEY,
  LANG_KEY,
  BAR_KEY,
  HIST_MODE_KEY,
  HIST_DAYS_KEY,
  HIST_PAGE_LIMIT,
  POLL_BASE,
  POLL_CHARGE,
  POLL_MAX,
  CHART_W,
  CHART_H,
  CHANNEL_KEY,
  REASON_KEY,
  fmtTs,
  fmtRange,
  fmtInt,
  fmtFix,
  fmtGroup,
  dayStart,
  fmtClock,
  fmtDayLabel,
  fmtShortDate,
  contentFP,
  chargeActive,
  storedThemeMode,
  storedAmoled,
  hostColorSupported,
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
