import { useEffect } from 'preact/hooks';

// 顶栏折叠只动画 transform（背景 scaleY + 标题 scale/translate）：
// 不支持 Scroll-driven Animations 时由 scroll + rAF 写 CSS 变量兜底；
// 支持时交给合成线程，主线程不再逐帧改样式。
const COLLAPSE_RANGE = 72;
const BG_END = 0.58;
const TITLE_END_SCALE = 20 / 28;
const TITLE_END_Y = -39;

function useCollapsingTopbar() {
  useEffect(function () {
    const root = document.documentElement;
    const scroller = document.getElementById('app');
    const scrollTarget = scroller || window;
    const mq = window.matchMedia ? window.matchMedia('(prefers-reduced-motion: reduce)') : null;
    const supportsScrollTimeline = !!(
      window.CSS &&
      window.CSS.supports &&
      window.CSS.supports('animation-timeline: --cre-page') &&
      window.CSS.supports('scroll-timeline-name: --cre-page')
    );
    let raf = 0;
    let reduced = !!(mq && mq.matches);
    let scrolled = null;

    if (supportsScrollTimeline) root.classList.add('tb-scroll-driven');
    else root.classList.add('tb-fallback');

    function markScrolled(isScrolled) {
      if (isScrolled === scrolled) return;
      scrolled = isScrolled;
      root.setAttribute('data-scrolled', isScrolled ? '1' : '0');
    }

    function apply() {
      raf = 0;
      const y = scroller ? scroller.scrollTop : window.scrollY || window.pageYOffset || 0;
      markScrolled(y > 2);
      if (reduced || supportsScrollTimeline) return;
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
    scrollTarget.addEventListener('scroll', schedule, { passive: true });
    window.addEventListener('resize', schedule);
    if (mq) {
      if (mq.addEventListener) mq.addEventListener('change', onMotionChange);
      else if (mq.addListener) mq.addListener(onMotionChange);
    }
    return function () {
      scrollTarget.removeEventListener('scroll', schedule);
      window.removeEventListener('resize', schedule);
      root.classList.remove('tb-scroll-driven', 'tb-fallback');
      if (mq) {
        if (mq.removeEventListener) mq.removeEventListener('change', onMotionChange);
        else if (mq.removeListener) mq.removeListener(onMotionChange);
      }
      if (raf) cancelAnimationFrame(raf);
    };
  }, []);
}

export { useCollapsingTopbar };
