'use strict';

var CRE_VENDOR_OK = !!(window.preact && window.preactHooks);
var h = CRE_VENDOR_OK ? window.preact.h : null;
var render = CRE_VENDOR_OK ? window.preact.render : function () {};
var createContext = CRE_VENDOR_OK ? window.preact.createContext : function (v) { return { value: v }; };
var useState = CRE_VENDOR_OK ? window.preactHooks.useState : function (v) { return [v, function () {}]; };
var useEffect = CRE_VENDOR_OK ? window.preactHooks.useEffect : function () {};
var useRef = CRE_VENDOR_OK ? window.preactHooks.useRef : function (v) { return { current: v }; };
var useCallback = CRE_VENDOR_OK ? window.preactHooks.useCallback : function (f) { return f; };
var useContext = CRE_VENDOR_OK ? window.preactHooks.useContext : function (ctx) { return ctx && ctx.value ? ctx.value : { lang: 'zh', t: function (k) { return k; } }; };
if (!CRE_VENDOR_OK) document.getElementById('root').innerHTML = '<div style="padding:24px;color:#fca5a5;font:14px system-ui">WebUI 资源加载失败（vendor/preact.umd.js 等）<br>Failed to load WebUI vendor scripts.</div>';

var CMD_JSON = '/data/adb/modules/CRecordEvolution/bin/batteryd json';
  var CMD_HISTORY = '/data/adb/modules/CRecordEvolution/bin/batteryd history';
  var CMD_ONCE = '/data/adb/modules/CRecordEvolution/bin/batteryd once';
  var LANG_FILE = '/data/adb/modules/CRecordEvolution/data/lang';
  var THEME_KEY = 'cre_theme';
  var LANG_KEY = 'cre_lang';
  var BAR_KEY = 'cre_barmode';
  var HIST_MODE_KEY = 'cre_hist_mode';
  var HIST_DAYS_KEY = 'cre_hist_days';
  var HIST_PAGE_LIMIT = 20;

  var POLL_BASE = 60000, POLL_MAX = 300000, TIMEOUT_MS = 15000;
  var CHART_W = 320, CHART_H = 110;
  var MONTHS_EN = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
  var CHANNEL_KEY = { stable: 'channel_stable', ml: 'channel_ml' };
  var REASON_KEY = {
    temp_out_of_range: 'reason_temp',
    delta_lt_20: 'reason_delta',
    out_of_window: 'reason_window',
    outlier: 'reason_outlier'
  };

  

function pad(n) { n = String(n); return n.length < 2 ? '0' + n : n; }

  function fmtTs(lang, ts) {    var d = new Date(Number(ts) * 1000);
    if (isNaN(d.getTime())) return '--';
    if (lang === 'en') {
      return MONTHS_EN[d.getMonth()] + ' ' + d.getDate() + ' ' + pad(d.getHours()) + ':' + pad(d.getMinutes());
    }
    return pad(d.getMonth() + 1) + '-' + pad(d.getDate()) + ' ' + pad(d.getHours()) + ':' + pad(d.getMinutes());
  }

  function fmtRange(lang, startTs, endTs) {
    if (!startTs) return '--';
    if (!endTs) return fmtTs(lang, startTs);
    var a = new Date(Number(startTs) * 1000), b = new Date(Number(endTs) * 1000);
    if (isNaN(a.getTime()) || isNaN(b.getTime())) return '--';
    var sameDay = a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
    if (lang === 'en') {
      var head = MONTHS_EN[a.getMonth()] + ' ' + a.getDate() + ' ' + pad(a.getHours()) + ':' + pad(a.getMinutes());
      var tail = sameDay
        ? pad(b.getHours()) + ':' + pad(b.getMinutes())
        : MONTHS_EN[b.getMonth()] + ' ' + b.getDate() + ' ' + pad(b.getHours()) + ':' + pad(b.getMinutes());
      return head + '–' + tail;
    }
    var zhHead = pad(a.getMonth() + 1) + '-' + pad(a.getDate()) + ' ' + pad(a.getHours()) + ':' + pad(a.getMinutes());
    var zhTail = sameDay
      ? pad(b.getHours()) + ':' + pad(b.getMinutes())
      : pad(b.getMonth() + 1) + '-' + pad(b.getDate()) + ' ' + pad(b.getHours()) + ':' + pad(b.getMinutes());
    return zhHead + '~' + zhTail;
  }

  function fmtInt(v) { return v == null ? '—' : String(v); }
  function fmtFix(v, n) { return v == null ? '—' : Number(v).toFixed(n); }

  function dayStart(ts) {
    var d = new Date(Number(ts) * 1000);
    if (isNaN(d.getTime())) return 0;
    return new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
  }

  function fmtClock(lang, ts) {
    var d = new Date(Number(ts) * 1000);
    if (isNaN(d.getTime())) return '--';
    return pad(d.getHours()) + ':' + pad(d.getMinutes());
  }

  function fmtDayLabel(lang, ts) {
    var d = new Date(Number(ts) * 1000);
    if (isNaN(d.getTime())) return '--';
    var today = dayStart(Date.now() / 1000);
    var day = dayStart(ts);
    var diff = Math.round((today - day) / 86400000);
    if (diff === 0) return translate(lang, 'hist_today');
    if (diff === 1) return translate(lang, 'hist_yesterday');
    var now = new Date();
    if (lang === 'en') {
      return MONTHS_EN[d.getMonth()] + ' ' + d.getDate() + (d.getFullYear() !== now.getFullYear() ? ', ' + d.getFullYear() : '');
    }
    return (d.getFullYear() !== now.getFullYear() ? d.getFullYear() + '年' : '') + (d.getMonth() + 1) + '月' + d.getDate() + '日';
  }

  function fmtShortDate(lang, ts) {
    var d = new Date(Number(ts) * 1000);
    if (isNaN(d.getTime())) return '--';
    return lang === 'en' ? MONTHS_EN[d.getMonth()] + ' ' + d.getDate() : (d.getMonth() + 1) + '/' + d.getDate();
  }

  function normalize(a, b, c) {
    if (a && typeof a === 'object' && ('errno' in a || 'stdout' in a)) return a;
    return { errno: Number(a) || 0, stdout: b == null ? '' : String(b), stderr: c == null ? '' : String(c) };
  }

  function pickHost() {
    if (typeof ksu !== 'undefined' && ksu && typeof ksu.exec === 'function') return ksu;
    if (window.APatch && typeof window.APatch.exec === 'function') return window.APatch;
    if (window.apatch && typeof window.apatch.exec === 'function') return window.apatch;
    return null;
  }

  function hasExec() { return pickHost() != null; }

  function requestEdgeToEdge() {
    // 不依赖 exec 是否存在：即使宿主只暴露了 edge-to-edge 接口也直接调用。
    try {
      if (typeof ksu !== 'undefined' && ksu) {
        if (typeof ksu.enableEdgeToEdge === 'function') { ksu.enableEdgeToEdge(true); return; }
        if (typeof ksu.enableInsets === 'function') { ksu.enableInsets(true); return; }
      }
    } catch (e) { /* 老宿主没有该接口 */ }
    var host = pickHost();
    if (!host) return;
    try {
      if (typeof host.enableEdgeToEdge === 'function') { host.enableEdgeToEdge(true); return; }
      if (typeof host.enableInsets === 'function') host.enableInsets(true);
    } catch (e) { /* 老宿主没有该接口 */ }
  }

  function cbNameExec(host, cmd) {
    return new Promise(function (resolve) {
      var name = '__cr_cb_' + Date.now() + '_' + Math.floor(Math.random() * 1e6);
      var done = false;
      window[name] = function (errno, stdout, stderr) {
        if (done) return;
        done = true;
        try { delete window[name]; } catch (e) { window[name] = undefined; }
        resolve(normalize(errno, stdout, stderr));
      };
      try {
        host.exec(cmd, '{}', name);
      } catch (e) {
        if (!done) { done = true; resolve({ errno: -1, stdout: '', stderr: String(e) }); }
      }
    });
  }

  function promiseExec(host, cmd) {
    return new Promise(function (resolve) {
      var p;
      try { p = host.exec(cmd); } catch (e) { resolve(null); return; }
      if (p && typeof p.then === 'function') {
        p.then(function (r) { resolve(r && typeof r === 'object' ? normalize(r) : null); },
               function () { resolve(null); });
      } else { resolve(null); }
    });
  }

  function directCbExec(host, cmd) {
    return new Promise(function (resolve) {
      try {
        host.exec(cmd, function (a, b, c) { resolve(normalize(a, b, c)); });
      } catch (e) { resolve(null); }
    });
  }

  function run(cmd) {
    var host = pickHost();
    if (!host) return Promise.resolve({ errno: -1, nohost: true, stdout: '', stderr: '' });
    var kinds = [cbNameExec, promiseExec, directCbExec];
    return new Promise(function (resolve) {
      var settled = false;
      var timer = setTimeout(function () {
        if (!settled) { settled = true; resolve({ errno: -1, stdout: '', stderr: 'timeout' }); }
      }, TIMEOUT_MS);
      function attempt(i) {
        if (settled) return;
        kinds[i](host, cmd).then(function (r) {
          if (settled) return;
          if (r && Number(r.errno) !== -1) { settled = true; clearTimeout(timer); resolve(r); return; }
          if (i + 1 < kinds.length) { setTimeout(function () { attempt(i + 1); }, 350); return; }
          settled = true; clearTimeout(timer); resolve({ errno: -1, stdout: '', stderr: '' });
        });
      }
      attempt(0);
    });
  }

  var langSyncBusy = false;
  var langPending = null;
  function syncModuleLang(lang) {
    // 连续快速切换时排队最后一次，避免旧写请求先完成覆盖新选择
    if (langSyncBusy) { langPending = lang; return; }
    langSyncBusy = true;
    var cmd = "printf '%s\\n' " + lang + " > " + LANG_FILE + ".tmp && mv " + LANG_FILE + ".tmp " + LANG_FILE;
    run(cmd).then(function () {
      langSyncBusy = false;
      if (langPending) {
        var next = langPending;
        langPending = null;
        syncModuleLang(next);
        return;
      }
      run(CMD_ONCE);
    }, function () {
      langSyncBusy = false;
      langPending = null;
    });
  }

  