import { render } from 'preact';
import { useState, useEffect, useRef, useCallback } from 'preact/hooks';
import { translate } from './i18n.js';
import {
  CMD_JSON,
  CMD_ONCE,
  LANG_FILE,
  LANG_KEY,
  THEME_KEY,
  BAR_KEY,
  POLL_BASE,
  POLL_MAX,
  run,
  syncModuleLang,
  hasExec,
  requestEdgeToEdge,
  contentFP,
  storedThemeMode,
  storedBarMode,
  applyThemeDocument,
  initialLang,
} from './core.js';
import { LangCtx, TopBar, Overview, Settings, Dock, Gate, ErrorBanner } from './ui/index.js';
import { History } from './history/index.js';
import './app.css';
const HAS_HOST = hasExec();
function App() {
  const [lang, setLang] = useState(initialLang());
  const [themeMode, setThemeMode] = useState(storedThemeMode());
  const [barMode, setBarMode] = useState(storedBarMode());
  const [page, setPage] = useState(0);
  const [doc, setDoc] = useState(null);
  const [err, setErr] = useState(null);
  const [busy, setBusy] = useState(false);
  const [histReload, setHistReload] = useState(0);
  const scrollPos = useRef([0, 0, 0]);
  const lastFP = useRef(null);
  const fetchInFlight = useRef(null);
  const pollCtl = useRef({
    reset: null,
  });
  const tr = function (key, vars) {
    return translate(lang, key, vars);
  };
  useEffect(
    function () {
      document.documentElement.setAttribute('data-lang', lang);
      document.documentElement.lang = lang === 'zh' ? 'zh-CN' : 'en';
      document.title =
        page === 0
          ? 'ChargingRecord Evolution'
          : translate(lang, page === 1 ? 'page_history' : 'page_settings') +
            ' · ChargingRecord Evolution';
    },
    [lang, page]
  );
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
  useEffect(
    function () {
      document.documentElement.setAttribute('data-bar', barMode);
    },
    [barMode]
  );
  useEffect(
    function () {
      try {
        window.scrollTo(0, scrollPos.current[page] || 0);
      } catch (e) {}
    },
    [page]
  );
  const fetchDoc = useCallback(function (force) {
    if (document.hidden) return Promise.resolve(true);
    if (fetchInFlight.current) {
      if (!force) return fetchInFlight.current;
      return fetchInFlight.current.then(function () {
        return fetchDoc(true);
      });
    }
    const p = run(CMD_JSON).then(function (r) {
      if (r.errno !== 0) {
        setErr({
          key: 'err_run',
          vars: {
            detail: r.stderr || 'errno ' + r.errno,
          },
        });
        return false;
      }
      const stdout = String(r.stdout || '').trim();
      if (!stdout) {
        setErr({
          key: 'err_noout',
        });
        return false;
      }
      let data;
      try {
        data = JSON.parse(stdout);
      } catch (e) {
        setErr({
          key: 'err_json',
        });
        return false;
      }
      if (!data || typeof data !== 'object') {
        setErr({
          key: 'err_struct',
        });
        return false;
      }
      const fp = contentFP(data);
      if (!force && fp === lastFP.current) {
        setDoc(data);
        setErr(null);
        return true;
      }
      lastFP.current = fp;
      setDoc(data);
      setErr(null);
      return true;
    });
    fetchInFlight.current = p.then(
      function (ok) {
        fetchInFlight.current = null;
        return ok;
      },
      function (e) {
        fetchInFlight.current = null;
        setErr({
          key: 'err_render',
          vars: {
            msg: String((e && e.message) || e),
          },
        });
        return false;
      }
    );
    return fetchInFlight.current;
  }, []);
  useEffect(
    function () {
      if (!HAS_HOST) return;
      let timer = null,
        delay = 0;
      function tick() {
        timer = setTimeout(function () {
          timer = null;
          fetchDoc(false).then(function (ok) {
            delay = ok ? POLL_BASE : Math.min((delay || POLL_BASE) * 2, POLL_MAX);
            tick();
          });
        }, delay);
      }
      function reset() {
        if (timer) {
          clearTimeout(timer);
          timer = null;
        }
        delay = 0;
        tick();
      }
      pollCtl.current.reset = reset;
      tick();
      function onVisibility() {
        if (!document.hidden) reset();
      }
      document.addEventListener('visibilitychange', onVisibility);
      return function () {
        if (timer) clearTimeout(timer);
        document.removeEventListener('visibilitychange', onVisibility);
        pollCtl.current.reset = null;
      };
    },
    [fetchDoc]
  );
  useEffect(function () {
    if (!HAS_HOST) return;
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
  function goPage(i) {
    if (i === page) {
      try {
        window.scrollTo({
          top: 0,
          behavior: 'smooth',
        });
      } catch (e) {
        window.scrollTo(0, 0);
      }
      return;
    }
    scrollPos.current[page] = window.scrollY || 0;
    setPage(i);
  }
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
  function refreshNow() {
    if (busy) return;
    setBusy(true);
    run(CMD_ONCE).then(function () {
      setTimeout(function () {
        fetchDoc(true).then(function () {
          if (pollCtl.current.reset) pollCtl.current.reset();
          setHistReload(function (n) {
            return n + 1;
          });
          setBusy(false);
        });
      }, 800);
    });
  }
  if (!HAS_HOST) {
    return (
      <LangCtx.Provider
        value={{
          lang: lang,
          t: tr,
        }}
      >
        <TopBar page={0} busy={false} gateMode onRefresh={null} />
        <main>
          <div class="pagewrap">
            <Gate />
          </div>
        </main>
      </LangCtx.Provider>
    );
  }
  return (
    <LangCtx.Provider
      value={{
        lang: lang,
        t: tr,
      }}
    >
      <TopBar page={page} busy={busy} gateMode={false} onRefresh={refreshNow} />
      <main>
        <div class="pagewrap">
          <ErrorBanner err={err} />
          <Overview doc={doc} active={page === 0} />
          <History active={page === 1} reloadKey={histReload} />
          <Settings
            doc={doc}
            active={page === 2}
            themeMode={themeMode}
            barMode={barMode}
            busy={busy}
            onThemeChange={changeTheme}
            onLangChange={changeLang}
            onBarChange={changeBar}
            onRefresh={refreshNow}
          />
        </div>
      </main>
      <Dock page={page} onNavigate={goPage} />
    </LangCtx.Provider>
  );
}
requestEdgeToEdge();
setTimeout(requestEdgeToEdge, 350);
window.addEventListener('load', requestEdgeToEdge);
render(<App />, document.getElementById('root'));
