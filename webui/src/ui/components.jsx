import { toChildArray } from 'preact';
import { useEffect, useRef } from 'preact/hooks';
import { translate } from '../i18n.js';
import { useTr } from './context.js';

function Icon({ name }) {
  return <span class={'icon icon-' + name} aria-hidden="true" />;
}

function ErrorBanner(props) {
  const { t, lang } = useTr();
  if (!props.err) return null;
  return (
    <section class="err" role="alert">
      {translate(lang, props.err.key, props.err.vars)}
    </section>
  );
}

function TopBar(props) {
  const { t, lang } = useTr();
  const titles = [
    'CRecord Evolution',
    translate(lang, 'page_history'),
    translate(lang, 'page_settings'),
  ];
  return (
    <header class="topbar">
      <div class="topbar-status" aria-hidden="true" />
      <div class="topbar-bg" aria-hidden="true" />
      <div class="topbar-inner">
        <div class="topbar-titles">
          <div class="topbar-title-track" id="titleTrack">
            {titles.map(function (title, i) {
              return (
                <h1
                  key={title}
                  class="topbar-title"
                  id={props.page === i ? 'pageTitle' : null}
                  aria-hidden={props.page === i ? 'false' : 'true'}
                >
                  {title}
                </h1>
              );
            })}
          </div>
        </div>
      </div>
    </header>
  );
}

function Dock(props) {
  const { t, lang } = useTr();
  const innerRef = useRef(null);
  const navRef = useRef(props.onNavigate);
  const pageRef = useRef(props.page);
  navRef.current = props.onNavigate;
  pageRef.current = props.page;
  const dragRef = useRef(null);
  const suppressClick = useRef(false);

  // 手指直接在底栏上左右拖动：实时带动横向 pager，松手平滑吸附到最近页。
  // 触摸走 touch 事件（Android WebView 更可靠），鼠标走 pointer 事件，避免两边重复。
  useEffect(function () {
    const inner = innerRef.current;
    const app = document.getElementById('app');
    if (!inner || !app) return undefined;
    let snapTimer = 0;

    function slotWidth() {
      return inner.clientWidth / 3 || 1;
    }

    function startDrag(x, id) {
      clearTimeout(snapTimer);
      dragRef.current = {
        id: id,
        x: x,
        startPage: pageRef.current,
        progress: pageRef.current,
        moved: false,
      };
    }

    function moveDrag(x) {
      const drag = dragRef.current;
      if (!drag) return false;
      const dx = x - drag.x;
      if (!drag.moved && Math.abs(dx) < 6) return false;
      drag.moved = true;
      suppressClick.current = true;
      const width = app.clientWidth || 1;
      const progress = Math.max(0, Math.min(2, drag.startPage + dx / slotWidth()));
      drag.progress = progress;
      app.style.scrollSnapType = 'none';
      app.scrollLeft = progress * width;
      return true;
    }

    // 松手时保持 scrollSnapType:none 完成平滑滚动，结束后再恢复；否则恢复 snap
    // 会先把 scrollLeft 瞬间吸到最近页，看起来像没有切换动画。
    function settleDrag(cancelled) {
      const drag = dragRef.current;
      if (!drag) return;
      dragRef.current = null;
      if (!drag.moved) {
        suppressClick.current = false;
        return;
      }
      const width = app.clientWidth || 1;
      const target = cancelled
        ? pageRef.current
        : Math.max(0, Math.min(2, Math.round(drag.progress)));
      app.style.scrollSnapType = 'none';
      if (target === pageRef.current) {
        app.scrollTo({ left: target * width, behavior: 'smooth' });
        clearTimeout(snapTimer);
        snapTimer = setTimeout(function () {
          app.style.scrollSnapType = '';
        }, 380);
      } else {
        // 切页交给 App 的 Miuix 弹簧动画，snap 由它在动画结束时恢复。
        clearTimeout(snapTimer);
        navRef.current(target);
      }
      setTimeout(function () {
        suppressClick.current = false;
      }, 0);
    }

    function onPointerDown(event) {
      if (event.pointerType === 'touch') return;
      if (event.pointerType === 'mouse' && event.button !== 0) return;
      startDrag(event.clientX, event.pointerId);
    }

    function onPointerMove(event) {
      if (event.pointerType === 'touch') return;
      const drag = dragRef.current;
      if (!drag || drag.id !== event.pointerId) return;
      if (moveDrag(event.clientX)) event.preventDefault();
    }

    function onPointerUp(event) {
      if (event.pointerType === 'touch') return;
      const drag = dragRef.current;
      if (!drag || drag.id !== event.pointerId) return;
      settleDrag(false);
    }

    function onPointerCancel(event) {
      if (event.pointerType === 'touch') return;
      const drag = dragRef.current;
      if (!drag || drag.id !== event.pointerId) return;
      settleDrag(true);
    }

    function onTouchStart(event) {
      if (event.touches.length !== 1) return;
      startDrag(event.touches[0].clientX, 'touch');
    }

    function onTouchMove(event) {
      const drag = dragRef.current;
      if (!drag || drag.id !== 'touch' || event.touches.length !== 1) return;
      if (moveDrag(event.touches[0].clientX)) event.preventDefault();
    }

    function onTouchEnd(event) {
      const drag = dragRef.current;
      if (!drag || drag.id !== 'touch') return;
      // 只有真正拖动过才 preventDefault；普通点按必须保留 click，否则底栏不响应单击。
      if (drag.moved && event.cancelable) event.preventDefault();
      settleDrag(false);
    }

    function onTouchCancel() {
      const drag = dragRef.current;
      if (!drag || drag.id !== 'touch') return;
      settleDrag(true);
    }

    function onClickCapture(event) {
      if (!suppressClick.current) return;
      event.preventDefault();
      event.stopPropagation();
    }

    inner.addEventListener('pointerdown', onPointerDown);
    window.addEventListener('pointermove', onPointerMove, { passive: false });
    window.addEventListener('pointerup', onPointerUp);
    window.addEventListener('pointercancel', onPointerCancel);
    inner.addEventListener('touchstart', onTouchStart, { passive: true });
    inner.addEventListener('touchmove', onTouchMove, { passive: false });
    inner.addEventListener('touchend', onTouchEnd, { passive: false });
    inner.addEventListener('touchcancel', onTouchCancel, { passive: true });
    inner.addEventListener('click', onClickCapture, true);
    return function () {
      inner.removeEventListener('pointerdown', onPointerDown);
      window.removeEventListener('pointermove', onPointerMove);
      window.removeEventListener('pointerup', onPointerUp);
      window.removeEventListener('pointercancel', onPointerCancel);
      inner.removeEventListener('touchstart', onTouchStart);
      inner.removeEventListener('touchmove', onTouchMove);
      inner.removeEventListener('touchend', onTouchEnd);
      inner.removeEventListener('touchcancel', onTouchCancel);
      inner.removeEventListener('click', onClickCapture, true);
      clearTimeout(snapTimer);
    };
  }, []);

  const items = [
    { id: 'overview', label: 'nav_overview' },
    { id: 'history', label: 'nav_history' },
    { id: 'settings', label: 'nav_settings' },
  ];
  return (
    <nav class="dock" aria-label={t('nav_aria')}>
      <div class="dock-inner" role="tablist" ref={innerRef}>
        <span id="dockInd" class="dock-ind" aria-hidden="true" />
        {items.map(function (item, i) {
          return (
            <button
              key={item.id}
              id={'nav_' + item.id}
              data-page={i}
              class={'navitem' + (props.page === i ? ' is-active' : '')}
              type="button"
              role="tab"
              aria-selected={props.page === i ? 'true' : 'false'}
              onClick={function () {
                props.onNavigate(i);
              }}
            >
              <Icon name={item.id} />
              <span>{translate(lang, item.label)}</span>
            </button>
          );
        })}
      </div>
    </nav>
  );
}

function Seg(props) {
  return (
    <div class="seg" id={props.id || null} role="group" aria-label={props.label || null}>
      {props.options.map(function (o) {
        return (
          <button
            key={o.value}
            type="button"
            class={'seg-btn' + (o.value === props.value ? ' is-active' : '')}
            aria-pressed={o.value === props.value ? 'true' : 'false'}
            onClick={function () {
              props.onChange(o.value);
            }}
          >
            {o.label}
          </button>
        );
      })}
    </div>
  );
}

function SetStack(props) {
  return (
    <div class="set-item set-item--stack">
      <div class="set-text">
        <div class="set-title">{props.title}</div>
        <div class="set-desc">{props.desc}</div>
      </div>
      {props.control}
    </div>
  );
}

function SetRow(props) {
  return (
    <div class="set-item">
      <span class="set-title">{props.label}</span>
      <span
        class={'set-value' + (props.cls ? ' ' + props.cls : '')}
        data-field={props.field || null}
      >
        {props.value}
      </span>
    </div>
  );
}

function Divider() {
  return <div class="set-divider" />;
}

// SetGroup：设置页的卡片容器。子项之间自动插入 Divider，调用方不再手写 <Divider />。
// 选择数据驱动而不是 CSS .set-item:not(:last-child) 边框：.set-item 自带 14px/16px 内边距，
// 边框会横贯整个卡片，而现有 .set-divider 是左右各留 16px 的缩进分隔线，视觉不等价；
// 补新选择器需要改 app.css，因此仍在 DOM 里保留 .set-divider 节点。
function SetGroup(props) {
  const rows = [];
  toChildArray(props.children).forEach(function (child, i) {
    if (i > 0) rows.push(<Divider key={'divider-' + i} />);
    rows.push(child);
  });
  return <div class="set-group">{rows}</div>;
}

function Gate() {
  const { t, lang } = useTr();
  return (
    <section class="gate">
      <div class="gate-title">{t('gate_title')}</div>
      <div class="gate-desc">{t('gate_desc')}</div>
    </section>
  );
}

export { ErrorBanner, TopBar, Dock, Seg, SetStack, SetRow, SetGroup, Divider, Gate };
