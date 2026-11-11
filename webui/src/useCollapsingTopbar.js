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
      const scroller = document.querySelector('.page.is-active');
      const bg = document.querySelector('.topbar-bg');
      const titles = document.querySelector('.topbar-titles');
      if (!scroller || !bg || !titles) return undefined;
      const root = document.documentElement;
      const mq = window.matchMedia ? window.matchMedia('(prefers-reduced-motion: reduce)') : null;
      let raf = 0;
      let reduced = !!(mq && mq.matches);
      let scrolled = null;

      function markScrolled(isScrolled) {
        if (isScrolled === scrolled) return;
        scrolled = isScrolled;
        root.setAttribute('data-scrolled', isScrolled ? '1' : '0');
      }

      function apply() {
        raf = 0;
        const y = scroller.scrollTop || 0;
        markScrolled(y > 2);
        const p = reduced ? 1 : Math.max(0, Math.min(1, y / COLLAPSE_RANGE));
        const bgScale = 1 + (BG_END - 1) * p;
        const titleScale = 1 + (TITLE_END_SCALE - 1) * p;
        // 直接写元素 transform，避免每帧往 :root 写 CSS 变量触发全页样式重算。
        bg.style.transform = 'scale3d(1,' + bgScale + ',1)';
        titles.style.transform =
          'translate3d(0,' + TITLE_END_Y * p + 'px,0) scale(' + titleScale + ')';
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
