import { render } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import { translate } from './i18n.js';
import { hasExec, requestEdgeToEdge } from './core.js';
import { LangCtx, TopBar, Overview, Settings, Dock, Gate, ErrorBanner } from './ui/index.js';
import { History } from './history/index.js';
import { useDocPolling } from './useDocPolling.js';
import { useUiPrefs } from './useUiPrefs.js';
import './app.css';

const HAS_HOST_EXEC = hasExec();

// 两个渲染分支共用的外壳：LangCtx + 顶栏 + pagewrap；gate 分支没有底栏。
function AppShell({ value, topbar, dock, children }) {
  return (
    <LangCtx.Provider value={value}>
      {topbar}
      <main>
        <div class="pagewrap">{children}</div>
      </main>
      {dock}
    </LangCtx.Provider>
  );
}

// App 只负责「组合 hooks + 渲染壳」：取数与轮询在 useDocPolling，语言/主题/底栏
// 与其落盘在 useUiPrefs；这里保留页面切换、滚动位置、标题与 gate 分支。
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
  const scrollPos = useRef([0, 0, 0]);
  const tr = function (key, vars) {
    return translate(lang, key, vars);
  };
  const ctx = { lang, t: tr };

  useEffect(
    function () {
      document.title =
        page === 0
          ? 'ChargingRecord Evolution'
          : translate(lang, page === 1 ? 'page_history' : 'page_settings') +
            ' · ChargingRecord Evolution';
    },
    [lang, page]
  );

  // 切页后的滚动恢复：等 DOM 提交再等两帧，确保目标页（含历史页的列表/展开详情）
  // 完成布局后再 scrollTo；否则恢复会被同帧的骨架屏/高度抖动夹回 0。
  useEffect(
    function () {
      const y = scrollPos.current[page] || 0;
      let raf1 = 0,
        raf2 = 0;
      raf1 = requestAnimationFrame(function () {
        raf2 = requestAnimationFrame(function () {
          try {
            window.scrollTo(0, y);
          } catch (e) {}
        });
      });
      return function () {
        cancelAnimationFrame(raf1);
        cancelAnimationFrame(raf2);
      };
    },
    [page]
  );

  function goPage(i) {
    if (i === page) {
      try {
        window.scrollTo({ top: 0, behavior: 'smooth' });
      } catch (e) {
        window.scrollTo(0, 0);
      }
      return;
    }
    scrollPos.current[page] = window.scrollY || 0;
    setPage(i);
  }

  if (!HAS_HOST_EXEC) {
    return (
      <AppShell value={ctx} topbar={<TopBar page={0} busy={false} gateMode onRefresh={null} />}>
        <Gate />
      </AppShell>
    );
  }

  return (
    <AppShell
      value={ctx}
      topbar={<TopBar page={page} busy={busy} gateMode={false} onRefresh={refreshNow} />}
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
