import { render } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import { translate } from './i18n.js';
import { hasExec, requestEdgeToEdge } from './core.js';
import { LangCtx, TopBar, Overview, Settings, Dock, Gate, ErrorBanner } from './ui/index.js';
import { History } from './history/index.js';
import { useDocPolling } from './useDocPolling.js';
import { useUiPrefs } from './useUiPrefs.js';
import { useCollapsingTopbar } from './useCollapsingTopbar.js';
import './app.css';

const HAS_HOST_EXEC = hasExec();

// 外层 #app 是横向 ViewPager 视口；每个 .page 自己纵向滚动。
// 这样手指横滑时页面实时跟手、松手由原生 scroll-snap 回弹，接近 LSPosed 的 ViewPager2。
function appScroller() {
  return document.getElementById('app');
}

function activePageScroller() {
  return document.querySelector('.page.is-active');
}

function AppShell({ value, topbar, dock, children, single }) {
  return (
    <LangCtx.Provider value={value}>
      <div id="app" class={'app-scroll' + (single ? ' is-single' : '')}>
        {topbar}
        <div id="pager-track">{children}</div>
      </div>
      {dock}
    </LangCtx.Provider>
  );
}

function App() {
  const {
    lang,
    themeMode,
    amoled,
    barMode,
    dynamicColor,
    setLang,
    setThemeMode,
    setAmoled,
    setBarMode,
  } = useUiPrefs();
  const { doc, err, busy, histReload, refreshNow } = useDocPolling();

  const [page, setPage] = useState(0);
  const pageRef = useRef(0);
  const tr = function (key, vars) {
    return translate(lang, key, vars);
  };
  const ctx = { lang, t: tr };
  useCollapsingTopbar(page);

  useEffect(
    function () {
      document.title =
        page === 0
          ? 'CRecord Evolution'
          : translate(lang, page === 1 ? 'page_history' : 'page_settings') + ' · CRecord Evolution';
    },
    [lang, page]
  );

  // 横向 pager：滚动过程中让底栏指示器实时跟手，停稳后再提交 page 状态。
  useEffect(function () {
    const el = appScroller();
    if (!el) return undefined;
    let raf = 0;
    let settle = 0;
    let current = pageRef.current;

    function apply() {
      raf = 0;
      const width = el.clientWidth || 1;
      const progress = Math.max(0, Math.min(2, el.scrollLeft / width));
      const indicator = document.getElementById('dockInd');
      if (indicator) indicator.style.transform = 'translateX(' + progress * 100 + '%)';
      clearTimeout(settle);
      settle = setTimeout(function () {
        const next = Math.max(0, Math.min(2, Math.round(progress)));
        if (next !== current) {
          current = next;
          pageRef.current = next;
          setPage(next);
        }
      }, 90);
    }

    function onScroll() {
      if (!raf) raf = requestAnimationFrame(apply);
    }

    apply();
    el.addEventListener('scroll', onScroll, { passive: true });
    window.addEventListener('resize', onScroll);
    return function () {
      el.removeEventListener('scroll', onScroll);
      window.removeEventListener('resize', onScroll);
      clearTimeout(settle);
      if (raf) cancelAnimationFrame(raf);
    };
  }, []);

  function goPage(i) {
    const next = Math.max(0, Math.min(2, i));
    if (next === pageRef.current) {
      const panel = activePageScroller();
      if (panel) panel.scrollTo({ top: 0, behavior: 'smooth' });
      return;
    }
    pageRef.current = next;
    setPage(next);
    const el = appScroller();
    if (el) {
      el.scrollTo({ left: next * el.clientWidth, behavior: 'smooth' });
    }
  }

  if (!HAS_HOST_EXEC) {
    return (
      <AppShell value={ctx} topbar={<TopBar page={0} />} single>
        <section class="page is-active">
          <Gate />
        </section>
      </AppShell>
    );
  }

  return (
    <AppShell
      value={ctx}
      topbar={<TopBar page={page} />}
      dock={<Dock page={page} onNavigate={goPage} />}
    >
      <ErrorBanner err={err} />
      <Overview doc={doc} active={page === 0} />
      <History active={page === 1} reloadKey={histReload} />
      <Settings
        doc={doc}
        active={page === 2}
        themeMode={themeMode}
        amoled={amoled}
        dynamicColor={dynamicColor}
        barMode={barMode}
        busy={busy}
        onThemeChange={setThemeMode}
        onAmoledChange={setAmoled}
        onLangChange={setLang}
        onBarChange={setBarMode}
        onRefresh={refreshNow}
      />
    </AppShell>
  );
}

requestEdgeToEdge();
setTimeout(requestEdgeToEdge, 350);
window.addEventListener('load', requestEdgeToEdge);

render(<App />, document.getElementById('root'));
