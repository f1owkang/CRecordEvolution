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

function Gate() {
  const { t, lang } = useTr();
  return (
    <section class="gate">
      <div class="gate-title">{t('gate_title')}</div>
      <div class="gate-desc">{t('gate_desc')}</div>
    </section>
  );
}

export { ErrorBanner, TopBar, Dock, Seg, SetStack, SetRow, Divider, Gate };
