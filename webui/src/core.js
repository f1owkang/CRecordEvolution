import { translate } from './i18n.js';
import { dimToContrast, ensureContrast, parseCssColor, relativeLuminance } from './contrast.js';
import { readPref } from './prefs.js';

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
// cbName 传输的观察窗：宿主 exec 既不回包又没有返回 thenable 时，等这么久仍无回调
// 就交棒给下一种传输（promiseExec/directCbExec），不必耗到总超时。
const CB_OBSERVE_MS = 4000;
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

// 宿主 exec 的三种常见协议：cbName（exec(cmd,'{}',函数名)，宿主按全局名回调）、
// promise（exec(cmd) 返回 Promise）、directCb（exec(cmd, 回调)）。依次尝试。
//
// cbName 的两个坑：
// 1. 有的宿主只返回 Promise 而不认第三个参数——返回 thenable 时直接采纳它的结果，
//    不再傻等回调；没有返回 thenable 才进观察窗，给回调型宿主留 CB_OBSERVE_MS。
// 2. window[name] 是全局回调，任何一次 settle（成功/失败/被放弃/总超时）都必须删掉，
//    否则泄漏在全局；run() 通过 setRelease 注册最终清理，覆盖总超时与后续传输成功。
function cbNameExec(host, cmd, setRelease) {
  return new Promise(function (resolve) {
    const name = '__cr_cb_' + Date.now() + '_' + Math.floor(Math.random() * 1e6);
    let done = false;
    let observeTimer = 0;

    function cleanup() {
      try {
        delete window[name];
      } catch (e) {
        window[name] = undefined;
      }
    }

    function finish(result) {
      if (done) return;
      done = true;
      if (observeTimer) {
        clearTimeout(observeTimer);
        observeTimer = 0;
      }
      cleanup();
      resolve(result);
    }

    // 最终 settle（总超时或后续传输成功）时由 run() 统一兜底清理。
    if (setRelease) setRelease(cleanup);
    window[name] = function (errno, stdout, stderr) {
      finish(normalize(errno, stdout, stderr));
    };

    let ret;
    try {
      ret = host.exec(cmd, '{}', name);
    } catch (e) {
      finish({ errno: -1, stdout: '', stderr: String(e) });
      return;
    }
    if (ret && typeof ret.then === 'function') {
      // promise-only 宿主：采纳返回值；null/非对象按 errno -1 交棒下一种传输。
      ret.then(
        function (r) {
          finish(r && typeof r === 'object' ? normalize(r) : { errno: -1, stdout: '', stderr: '' });
        },
        function () {
          finish({ errno: -1, stdout: '', stderr: '' });
        }
      );
      return;
    }
    // 回调型宿主可能在 exec 内同步回包，此时已经 finish，不能再留一个空转的观察窗计时器。
    if (done) return;
    // 回调型宿主：观察窗内回调优先（回调到达即 finish 并清掉计时器），窗内没回包就交棒。
    observeTimer = setTimeout(function () {
      observeTimer = 0;
      finish({ errno: -1, stdout: '', stderr: '' });
    }, CB_OBSERVE_MS);
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
    let timer = 0;
    const releases = [];

    function setRelease(fn) {
      if (typeof fn === 'function') releases.push(fn);
    }

    // 统一收尾：先跑完所有已注册的宿主侧清理（全局回调名），再交付最终结果；
    // 成功、失败、总超时、后续传输成功都走这里，因此不会有 window[name] 残留。
    function settle(result) {
      if (settled) return;
      settled = true;
      if (timer) {
        clearTimeout(timer);
        timer = 0;
      }
      while (releases.length) {
        const release = releases.pop();
        try {
          release();
        } catch (e) {
          /* 清理失败不改变传输结果 */
        }
      }
      resolve(result);
    }

    timer = setTimeout(function () {
      settle({ errno: -1, stdout: '', stderr: 'timeout' });
    }, TIMEOUT_MS);

    function attempt(i) {
      if (settled) return;
      kinds[i](host, cmd, setRelease).then(function (r) {
        if (settled) return;
        if (r && Number(r.errno) !== -1) {
          settle(r);
          return;
        }
        if (i + 1 < kinds.length) {
          setTimeout(function () {
            attempt(i + 1);
          }, 350);
          return;
        }
        settle({ errno: -1, stdout: '', stderr: '' });
      });
    }
    attempt(0);
  });
}

// 固定命令结果的统一校验：errno → stdout → JSON → 对象结构。返回 {doc} 或
// {error:{key,vars}}；错误 key 与 i18n 字典一致（err_run/err_noout/err_json/err_struct），
// 不会把 "empty/json/struct/errno" 这类裸 token 透给用户。
function parseJsonResult(result) {
  if (!result || Number(result.errno) !== 0) {
    const errno = result && result.errno != null ? result.errno : -1;
    const detail = (result && result.stderr) || 'errno ' + errno;
    return { error: { key: 'err_run', vars: { detail: detail } } };
  }
  const stdout = String(result.stdout || '').trim();
  if (!stdout) return { error: { key: 'err_noout' } };
  let doc;
  try {
    doc = JSON.parse(stdout);
  } catch (e) {
    return { error: { key: 'err_json' } };
  }
  if (!doc || typeof doc !== 'object') return { error: { key: 'err_struct' } };
  return { doc: doc };
}

// 执行一条固定命令并校验 JSON：概览轮询与历史页共用同一套错误模型。
function runJsonDoc(cmd) {
  return run(cmd).then(parseJsonResult);
}

let langSyncBusy = false;
let langPending = null;
function syncModuleLang(lang) {
  // 入参必须先过白名单：这是拼进 root shell 的命令，任何非 zh/en 值（如 'en; id'）
  // 直接丢弃，命令保持常量模板。
  if (lang !== 'zh' && lang !== 'en') return;
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
  return readPref(THEME_KEY, ['light', 'dark', 'my'], 'auto');
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
// 除空值外还要求 --background 可解析：paletteScheme() 依赖它的亮度推导明暗，
// 解析不了说明宿主注入的不是可用调色板（不能静默退回不可信的 matchMedia）。
function hostColorSupported() {
  try {
    const cs = getComputedStyle(document.documentElement);
    const primary = cs.getPropertyValue('--primary').trim();
    const background = cs.getPropertyValue('--background').trim();
    return !!(primary && parseCssColor(background));
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
  const rgb = parseCssColor(css);
  return rgb ? relativeLuminance(rgb) : null;
}

function paletteScheme() {
  const lum = cssColorLuminance(hostPaletteBackground());
  return lum == null ? systemTheme() : lum > 0.5 ? 'light' : 'dark';
}

// 纯黑背景是独立开关（cre_amoled），只在解析出的深色主题上叠加：
// 这样 auto 仍跟随系统，浅色不会被压黑，也无需把主题档位扩成四档。
function storedAmoled() {
  return readPref(AMOLED_KEY, ['1'], '0') === '1';
}

// 取色主题里 App token 与宿主角色 token 的映射（与 app.css 的 html[data-theme='my']
// 保持一致）。宿主调色板不保证角色之间符合规范对比度：KernelSU(Miuix) 与
// Monochrome 下 --tertiary 实测是 #000000、--secondary 是 #474747，图表线与
// 会话柱会直接融进 #0b0b0d 的卡片底色（issue #13）。应用主题时以卡片底色为
// 背景做一次纠偏，不足 4.5:1 就向 --onSurface 混合（保留色相），保证图表、
// 徽章与时间轴圆点可见；只在 data-theme='my' 生效，内置主题仍用 app.css 手选配色。
const PALETTE_CONTRAST_TOKENS = [
  ['--accent', '--primary'],
  ['--blue', '--tertiary'],
  ['--purple', '--tertiary'],
  ['--orange', '--secondary'],
  ['--rest-dot', '--onSurface'],
];
const PALETTE_MIN_CONTRAST = 4.5;

// 逐个候选读取可解析的颜色值：宿主缺某个角色时用下一个回退角色。
function paletteColorValue(cs, names) {
  for (let i = 0; i < names.length; i++) {
    const value = cs.getPropertyValue(names[i]).trim();
    if (parseCssColor(value)) return value;
  }
  return '';
}

function applyPaletteContrast(root) {
  const style = root.style;
  if (root.getAttribute('data-theme') !== 'my') {
    for (let i = 0; i < PALETTE_CONTRAST_TOKENS.length; i++) {
      style.removeProperty(PALETTE_CONTRAST_TOKENS[i][0]);
    }
    return;
  }
  const cs = getComputedStyle(root);
  // 图表/徽章都落在卡片上：--card 已经叠过 AMOLED 纯黑，优先读它。
  const bg = paletteColorValue(cs, ['--card', '--surfaceContainer', '--background']);
  const toward = paletteColorValue(cs, ['--onSurface', '--onBackground']);
  for (let i = 0; i < PALETTE_CONTRAST_TOKENS.length; i++) {
    const token = PALETTE_CONTRAST_TOKENS[i][0];
    const source = paletteColorValue(cs, [PALETTE_CONTRAST_TOKENS[i][1]]);
    const fixed = ensureContrast(source, bg, toward, PALETTE_MIN_CONTRAST);
    if (fixed) style.setProperty(token, fixed);
    else style.removeProperty(token);
  }
}

// 未选中柱的低强调色：由 --accent 朝 --card 混合到低强调档位。深色/纯黑主题
// 压到 3:1 下限（再暗就会融进卡片底）；浅色主题用更淡的 container 色调
// （1.8:1），让选中柱的深色成为唯一焦点，未选中柱退成背景层——无选中时仍全部
// 用满色系列色，所以默认状态的数据可读性不受影响。
const CHART_DIM_MIN_CONTRAST = 3;
const CHART_DIM_MIN_CONTRAST_LIGHT = 1.8;

function applyChartDim(root) {
  const cs = getComputedStyle(root);
  const bg = paletteColorValue(cs, ['--card', '--surfaceContainer', '--background']);
  const accent = paletteColorValue(cs, ['--accent']);
  const light = root.getAttribute('data-scheme') === 'light';
  const min = light ? CHART_DIM_MIN_CONTRAST_LIGHT : CHART_DIM_MIN_CONTRAST;
  const dim = dimToContrast(accent, bg, min);
  if (dim) root.style.setProperty('--chart-dim', dim);
  else root.style.removeProperty('--chart-dim');
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
  applyPaletteContrast(root);
  applyChartDim(root);
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
  return readPref(BAR_KEY, ['classic'], 'floating');
}

function storedHistMode() {
  return readPref(HIST_MODE_KEY, ['trends'], 'records');
}

function storedHistDays() {
  const s = Number(readPref(HIST_DAYS_KEY, ['7', '30', '90'], '30'));
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
  CB_OBSERVE_MS,
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
  runJsonDoc,
  parseJsonResult,
  copyText,
  syncModuleLang,
};
