import { translate } from '../i18n.js';
import { E, useTr } from './context.js';

function ErrorBanner(props) {
  const c = useTr();
  if (!props.err) return null;
  return E(
    'section',
    { class: 'err', role: 'alert' },
    translate(c.lang, props.err.key, props.err.vars)
  );
}

function IconRefresh() {
  return E(
    'svg',
    { viewBox: '0 0 24 24', 'aria-hidden': 'true' },
    E('path', { d: 'M20 11a8 8 0 1 0-2.3 5.7' }),
    E('path', { d: 'M20 5v6h-6' })
  );
}

function TopBar(props) {
  const c = useTr();
  const title =
    props.page === 0
      ? 'ChargingRecord Evolution'
      : translate(c.lang, props.page === 1 ? 'page_history' : 'page_settings');
  const showRefresh = !props.gateMode && props.page !== 2;
  return E(
    'header',
    { class: 'topbar' },
    E(
      'div',
      { class: 'topbar-inner' },
      E('h1', { id: 'pageTitle' }, title),
      showRefresh
        ? E(
            'button',
            {
              id: 'refreshTop',
              type: 'button',
              class: 'iconbtn' + (props.busy ? ' spinning' : ''),
              title: translate(c.lang, 'refresh'),
              'aria-label': translate(c.lang, 'refresh'),
              disabled: props.busy,
              onClick: props.onRefresh,
            },
            IconRefresh()
          )
        : null
    )
  );
}

function navIcon(name) {
  if (name === 'overview') {
    return E(
      'svg',
      { viewBox: '0 0 24 24', 'aria-hidden': 'true' },
      E('path', { d: 'M4 19V5' }),
      E('path', { d: 'M4 19h16' }),
      E('path', { d: 'M8 16l3.5-5 3 2.4L20 7' })
    );
  }
  if (name === 'history') {
    return E(
      'svg',
      { viewBox: '0 0 24 24', 'aria-hidden': 'true' },
      E('circle', { cx: '12', cy: '12', r: '8.5' }),
      E('path', { d: 'M12 7.5V12l3 2' })
    );
  }
  return E(
    'svg',
    { viewBox: '0 0 24 24', 'aria-hidden': 'true' },
    E('circle', { cx: '12', cy: '12', r: '3' }),
    E('path', {
      d: 'M19.4 15a1.7 1.7 0 0 0 .34 1.87l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.7 1.7 0 0 0-1.87-.34 1.7 1.7 0 0 0-1.03 1.56V21a2 2 0 1 1-4 0v-.09A1.7 1.7 0 0 0 9 19.35a1.7 1.7 0 0 0-1.87.34l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.7 1.7 0 0 0 4.64 15a1.7 1.7 0 0 0-1.56-1.03H3a2 2 0 1 1 0-4h.09A1.7 1.7 0 0 0 4.65 9a1.7 1.7 0 0 0-.34-1.87l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.7 1.7 0 0 0 9 4.64a1.7 1.7 0 0 0 1.03-1.56V3a2 2 0 1 1 4 0v.09A1.7 1.7 0 0 0 15 4.65a1.7 1.7 0 0 0 1.87-.34l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.7 1.7 0 0 0 19.36 9a1.7 1.7 0 0 0 1.56 1.03H21a2 2 0 1 1 0 4h-.09A1.7 1.7 0 0 0 19.4 15z',
    })
  );
}

function Dock(props) {
  const c = useTr();
  const items = [
    { id: 'overview', label: 'nav_overview' },
    { id: 'history', label: 'nav_history' },
    { id: 'settings', label: 'nav_settings' },
  ];
  return E(
    'nav',
    { class: 'dock', 'aria-label': c.t('nav_aria') },
    E(
      'div',
      { class: 'dock-inner', role: 'tablist' },
      E('span', {
        class: 'dock-ind',
        'aria-hidden': 'true',
        style: { transform: 'translateX(' + props.page * 100 + '%)' },
      }),
      items.map(function (item, i) {
        return E(
          'button',
          {
            key: item.id,
            id: 'nav_' + item.id,
            'data-page': i,
            class: 'navitem' + (props.page === i ? ' is-active' : ''),
            type: 'button',
            role: 'tab',
            'aria-selected': props.page === i ? 'true' : 'false',
            onClick: function () {
              props.onNavigate(i);
            },
          },
          navIcon(item.id),
          E('span', null, translate(c.lang, item.label))
        );
      })
    )
  );
}

function Seg(props) {
  return E(
    'div',
    { class: 'seg', id: props.id || null, role: 'group', 'aria-label': props.label || null },
    props.options.map(function (o) {
      return E(
        'button',
        {
          key: o.value,
          type: 'button',
          class: 'seg-btn' + (o.value === props.value ? ' is-active' : ''),
          onClick: function () {
            props.onChange(o.value);
          },
        },
        o.label
      );
    })
  );
}

function SetStack(props) {
  return E(
    'div',
    { class: 'set-item set-item--stack' },
    E(
      'div',
      { class: 'set-text' },
      E('div', { class: 'set-title' }, props.title),
      E('div', { class: 'set-desc' }, props.desc)
    ),
    props.control
  );
}

function SetRow(props) {
  return E(
    'div',
    { class: 'set-item' },
    E('span', { class: 'set-title' }, props.label),
    E(
      'span',
      {
        class: 'set-value' + (props.cls ? ' ' + props.cls : ''),
        'data-field': props.field || null,
      },
      props.value
    )
  );
}

function Divider() {
  return E('div', { class: 'set-divider' });
}

function Gate() {
  const c = useTr();
  return E(
    'section',
    { class: 'gate' },
    E('div', { class: 'gate-title' }, c.t('gate_title')),
    E('div', { class: 'gate-desc' }, c.t('gate_desc'))
  );
}

export { ErrorBanner, TopBar, Dock, Seg, SetStack, SetRow, Divider, Gate };
