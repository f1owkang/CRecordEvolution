import { useTr } from './context.js';
import { Seg, SetGroup, SetStack, SetRow } from './components.jsx';
import { CHANNEL_KEY, fmtInt } from '../core.js';

const THEME_OPTIONS = [
  { value: 'auto', key: 'theme_auto' },
  { value: 'light', key: 'theme_light' },
  { value: 'dark', key: 'theme_dark' },
];
const LANG_OPTIONS = [
  { value: 'zh', key: 'lang_zh' },
  { value: 'en', key: 'lang_en' },
];
const BAR_OPTIONS = [
  { value: 'floating', key: 'bar_floating' },
  { value: 'classic', key: 'bar_classic' },
];
const STORAGE_PATH = '/data/adb/modules/CRecordEvolution/data';

// Seg 的 options 只存 i18n key，渲染时再翻译，避免在模块级捕获 t。
function trOptions(options, t) {
  return options.map(function (o) {
    return { value: o.value, label: t(o.key) };
  });
}

// 设置页分区：标题 + 卡片；卡片子项间的 Divider 由 SetGroup 统一插入。
function SettingsSection(props) {
  return (
    <div class="settings-section">
      <div class="settings-title">{props.title}</div>
      <SetGroup>{props.children}</SetGroup>
    </div>
  );
}

function Settings(props) {
  const { t, lang } = useTr();
  const d = props.doc || {};
  const channelKey = CHANNEL_KEY[d.channel];
  const channel = channelKey ? t(channelKey) : d.channel || '—';

  // 外观：主题 / 语言 / 底栏三档分段选择器，只有取值与回调不同。
  const appearance = [
    {
      id: 'theme_seg',
      label: t('settings_theme'),
      desc: t('settings_theme_desc'),
      options: THEME_OPTIONS,
      value: props.themeMode,
      onChange: props.onThemeChange,
    },
    {
      id: 'lang_seg',
      label: t('settings_language'),
      desc: t('settings_language_desc'),
      options: LANG_OPTIONS,
      value: lang,
      onChange: props.onLangChange,
    },
    {
      id: 'bar_mode_seg',
      label: t('settings_bar_mode'),
      desc: t('settings_bar_mode_desc'),
      options: BAR_OPTIONS,
      value: props.barMode,
      onChange: props.onBarChange,
    },
  ];

  // 数据：字段名与 data-field 一一对应，仅存储路径行带额外 class。
  const dataRows = [
    { label: t('settings_channel'), value: channel, field: 'channel' },
    { label: t('settings_updated'), value: d.updated || '—', field: 'updated' },
    { label: t('settings_samples'), value: fmtInt(d.samples_n), field: 'samples' },
    {
      label: t('settings_sessions'),
      value: fmtInt(Array.isArray(d.sessions) ? d.sessions.length : null),
      field: 'sessions',
    },
    { label: t('settings_retention'), value: t('settings_retention_value') },
    { label: t('settings_storage'), value: STORAGE_PATH, cls: 'set-value--path' },
  ];

  return (
    <section class={'page' + (props.active ? ' is-active' : '')} id="page_settings" role="tabpanel">
      <SettingsSection title={t('settings_appearance')}>
        {appearance.map(function (row) {
          return (
            <SetStack
              key={row.id}
              title={row.label}
              desc={row.desc}
              control={
                <Seg
                  id={row.id}
                  label={row.label}
                  value={row.value}
                  options={trOptions(row.options, t)}
                  onChange={row.onChange}
                />
              }
            />
          );
        })}
      </SettingsSection>

      <SettingsSection title={t('settings_refresh')}>
        <SetStack
          title={t('settings_manual_refresh')}
          desc={t('settings_refresh_desc')}
          control={
            <button id="refresh" type="button" disabled={props.busy} onClick={props.onRefresh}>
              {props.busy ? t('refreshing') : t('refresh_now')}
            </button>
          }
        />
      </SettingsSection>

      <SettingsSection title={t('settings_data')}>
        {dataRows.map(function (row) {
          return (
            <SetRow
              key={row.field || row.label}
              label={row.label}
              value={row.value}
              field={row.field}
              cls={row.cls}
            />
          );
        })}
      </SettingsSection>

      <SettingsSection title={t('settings_about')}>
        <SetStack title="ChargingRecord Evolution" desc={t('settings_about_desc')} />
        <SetRow label={t('settings_webui')} value={t('settings_webui_value')} />
      </SettingsSection>
    </section>
  );
}
export { Settings };
