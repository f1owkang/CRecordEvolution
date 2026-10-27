import { E, useTr } from './context.js';
import { Seg, SetStack, SetRow, Divider } from './components.js';
import { CHANNEL_KEY, fmtInt } from '../core.js';

function Settings(props) {
  const c = useTr();
  const lang = c.lang;
  const d = props.doc || {};
  const channelKey = CHANNEL_KEY[d.channel];
  const channel = channelKey ? c.t(channelKey) : d.channel || '—';
  const themeOptions = [
    { value: 'auto', label: c.t('theme_auto') },
    { value: 'light', label: c.t('theme_light') },
    { value: 'dark', label: c.t('theme_dark') },
  ];
  const langOptions = [
    { value: 'zh', label: c.t('lang_zh') },
    { value: 'en', label: c.t('lang_en') },
  ];
  const barOptions = [
    { value: 'floating', label: c.t('bar_floating') },
    { value: 'classic', label: c.t('bar_classic') },
  ];
  return E(
    'section',
    { class: 'page' + (props.active ? ' is-active' : ''), id: 'page_settings', role: 'tabpanel' },
    E(
      'div',
      { class: 'settings-section' },
      E('div', { class: 'settings-title' }, c.t('settings_appearance')),
      E(
        'div',
        { class: 'set-group' },
        E(SetStack, {
          title: c.t('settings_theme'),
          desc: c.t('settings_theme_desc'),
          control: E(Seg, {
            id: 'theme_seg',
            label: c.t('settings_theme'),
            value: props.themeMode,
            options: themeOptions,
            onChange: props.onThemeChange,
          }),
        }),
        E(Divider),
        E(SetStack, {
          title: c.t('settings_language'),
          desc: c.t('settings_language_desc'),
          control: E(Seg, {
            id: 'lang_seg',
            label: c.t('settings_language'),
            value: lang,
            options: langOptions,
            onChange: props.onLangChange,
          }),
        }),
        E(Divider),
        E(SetStack, {
          title: c.t('settings_bar_mode'),
          desc: c.t('settings_bar_mode_desc'),
          control: E(Seg, {
            id: 'bar_mode_seg',
            label: c.t('settings_bar_mode'),
            value: props.barMode,
            options: barOptions,
            onChange: props.onBarChange,
          }),
        })
      )
    ),
    E(
      'div',
      { class: 'settings-section' },
      E('div', { class: 'settings-title' }, c.t('settings_refresh')),
      E(
        'div',
        { class: 'set-group' },
        E(SetStack, {
          title: c.t('settings_manual_refresh'),
          desc: c.t('settings_refresh_desc'),
          control: E(
            'button',
            {
              id: 'refresh',
              type: 'button',
              disabled: props.busy,
              onClick: props.onRefresh,
            },
            props.busy ? c.t('refreshing') : c.t('refresh_now')
          ),
        })
      )
    ),
    E(
      'div',
      { class: 'settings-section' },
      E('div', { class: 'settings-title' }, c.t('settings_data')),
      E(
        'div',
        { class: 'set-group' },
        E(SetRow, { label: c.t('settings_channel'), value: channel, field: 'channel' }),
        E(Divider),
        E(SetRow, { label: c.t('settings_updated'), value: d.updated || '—', field: 'updated' }),
        E(Divider),
        E(SetRow, { label: c.t('settings_samples'), value: fmtInt(d.samples_n), field: 'samples' }),
        E(Divider),
        E(SetRow, {
          label: c.t('settings_sessions'),
          value: fmtInt(Array.isArray(d.sessions) ? d.sessions.length : null),
          field: 'sessions',
        }),
        E(Divider),
        E(SetRow, { label: c.t('settings_retention'), value: c.t('settings_retention_value') }),
        E(Divider),
        E(SetRow, {
          label: c.t('settings_storage'),
          value: '/data/adb/modules/CRecordEvolution/data',
          cls: 'set-value--path',
        })
      )
    ),
    E(
      'div',
      { class: 'settings-section' },
      E('div', { class: 'settings-title' }, c.t('settings_about')),
      E(
        'div',
        { class: 'set-group' },
        E(
          'div',
          { class: 'set-item set-item--stack' },
          E(
            'div',
            { class: 'set-text' },
            E('div', { class: 'set-title' }, 'ChargingRecord Evolution'),
            E('div', { class: 'set-desc' }, c.t('settings_about_desc'))
          )
        ),
        E(Divider),
        E(SetRow, { label: c.t('settings_webui'), value: c.t('settings_webui_value') })
      )
    )
  );
}

export { Settings };
