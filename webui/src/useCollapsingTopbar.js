import { useEffect } from 'preact/hooks';

// 顶栏折叠只动画 transform（背景 scaleY + 标题 scale/translate）。
// 每个 .page 是独立纵向滚动面板，监听当前激活页；兼容不支持
// Scroll-driven Animations 的 WebView，不依赖 #app（横向 pager）滚动。
const COLLAPSE_RANGE = 72;
const BG_END = 0.58;
const TITLE_END_SCALE = 20 / 28;
const TITLE_END_Y = -39;

function useCollapsingTopbar(page) {
  useEffect(
    function () {
      const root = document.documentElement;
      const scroller = document.querySelector('.page.is-active');
      if (!scroller) return undefined;
      const mq = window.matchMedia ? window.matchMedia('(prefers-reduced-motion: reduce)') : null;
      let raf = 0;
      let reduced = !!(mq && mq.matches);
      let scrolled = null;

      root.classList.add('tb-fallback');

      function markScrolled(isScrolled) {
        if (isScrolled === scrolled) return;
        scrolled = isScrolled;
        root.setAttribute('data-scrolled', isScrolled ? '1' : '0');
      }

      function apply() {
        raf = 0;
        const y = scroller.scrollTop || 0;
        markScrolled(y > 2);
        if (reduced) return;
        const p = Math.max(0, Math.min(1, y / COLLAPSE_RANGE));
        root.style.setProperty('--tb-bg-scale', String(1 + (BG_END - 1) * p));
        root.style.setProperty('--tb-title-y', TITLE_END_Y * p + 'px');
        root.style.setProperty('--tb-title-scale', String(1 + (TITLE_END_SCALE - 1) * p));
      }

      function schedule() {
        if (!raf) raf = requestAnimationFrame(apply);
      }

      function onMotionChange() {
        reduced = !!(mq && mq.matches);
        apply();
      }

      apply();
      scroller.addEventListener('scroll', schedule, { passive: true });
      window.addEventListener('resize', schedule);
      if (mq) {
        if (mq.addEventListener) mq.addEventListener('change', onMotionChange);
        else if (mq.addListener) mq.addListener(onMotionChange);
      }
      return function () {
        scroller.removeEventListener('scroll', schedule);
        window.removeEventListener('resize', schedule);
        root.classList.remove('tb-fallback');
        if (mq) {
          if (mq.removeEventListener) mq.removeEventListener('change', onMotionChange);
          else if (mq.removeListener) mq.removeListener(onMotionChange);
        }
        if (raf) cancelAnimationFrame(raf);
      };
    },
    [page]
  );
}

export { useCollapsingTopbar };
