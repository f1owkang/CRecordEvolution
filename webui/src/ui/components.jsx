import { toChildArray } from 'preact';
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
  const title =
    props.page === 0
      ? 'ChargingRecord Evolution'
      : translate(lang, props.page === 1 ? 'page_history' : 'page_settings');
  const showRefresh = !props.gateMode && props.page !== 2;
  return (
    <header class="topbar">
      <div class="topbar-inner">
        <h1 id="pageTitle">{title}</h1>
        {showRefresh ? (
          <button
            id="refreshTop"
            type="button"
            class={'iconbtn' + (props.busy ? ' spinning' : '')}
            title={translate(lang, 'refresh')}
            aria-label={translate(lang, 'refresh')}
            disabled={props.busy}
            onClick={props.onRefresh}
          >
            <Icon name="refresh" />
          </button>
        ) : null}
      </div>
    </header>
  );
}

function Dock(props) {
  const { t, lang } = useTr();
  const items = [
    { id: 'overview', label: 'nav_overview' },
    { id: 'history', label: 'nav_history' },
    { id: 'settings', label: 'nav_settings' },
  ];
  return (
    <nav class="dock" aria-label={t('nav_aria')}>
      <div class="dock-inner" role="tablist">
        <span
          class="dock-ind"
          aria-hidden="true"
          style={{ transform: 'translateX(' + props.page * 100 + '%)' }}
        />
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
// 补新选择器又需要改 app.css，超出本次改动范围，故仍在 DOM 里保留 .set-divider 节点。
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
