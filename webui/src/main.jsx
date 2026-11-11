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
  const pagerAnim = useRef({ raf: 0, busy: false });
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
      // 标题横向 track 与页面滚动同一进度：滑到一半时两张标题各露一半。
      const titles = document.getElementById('titleTrack');
      if (titles) titles.style.transform = 'translate3d(' + -progress * (100 / 3) + '%,0,0)';
      const titleEls = document.querySelectorAll('.topbar-title');
      for (let i = 0; i < titleEls.length; i++) {
        titleEls[i].style.opacity = String(Math.max(0, 1 - Math.abs(progress - i)));
      }
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

  // Miuix PagerNavigationSpringSpec：stiffness 322.2、damping 32.31（mass=1）。
  // 原生 scrollTo smooth 的时长由 WebView 固定且偏慢，这里用同一弹簧参数自己积分，
  // 点击/底栏松手都能在约 0.3s 内完成同样的回弹手感。
  function animatePager(next) {
    const el = appScroller();
    if (!el) return;
    const anim = pagerAnim.current;
    if (anim.raf) cancelAnimationFrame(anim.raf);
    anim.raf = 0;
    const width = el.clientWidth || 1;
    const target = next * width;
    const reduced = !!(
      window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches
    );
    el.style.scrollSnapType = 'none';
    if (reduced || Math.abs(target - el.scrollLeft) < 1) {
      el.scrollLeft = target;
      el.style.scrollSnapType = '';
      anim.busy = false;
      return;
    }
    anim.busy = true;
    let x = el.scrollLeft;
    let v = 0;
    let last = performance.now();
    const K = 322.2;
    const C = 32.31;
    function step(now) {
      let dt = Math.min(0.032, Math.max(0, (now - last) / 1000));
      last = now;
      while (dt > 0) {
        const h = Math.min(1 / 240, dt);
        const a = -K * (x - target) - C * v;
        v += a * h;
        x += v * h;
        dt -= h;
      }
      el.scrollLeft = x;
      if (Math.abs(x - target) < 0.5 && Math.abs(v) < 8) {
        el.scrollLeft = target;
        el.style.scrollSnapType = '';
        anim.busy = false;
        anim.raf = 0;
        return;
      }
      anim.raf = requestAnimationFrame(step);
    }
    anim.raf = requestAnimationFrame(step);
  }

  function goPage(i) {
    const next = Math.max(0, Math.min(2, i));
    if (next === pageRef.current) {
      const panel = activePageScroller();
      if (panel) panel.scrollTo({ top: 0, behavior: 'smooth' });
      return;
    }
    pageRef.current = next;
    setPage(next);
    animatePager(next);
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
