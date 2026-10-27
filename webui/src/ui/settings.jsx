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
  return options.map(function (option) {
    return { value: option.value, label: t(option.key) };
  });
}

// 外观分区：主题 / 语言 / 底栏三档分段选择器，只有取值与回调不同。
function appearanceItems(props, t, lang) {
  const rows = [
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
  return rows.map(function (row) {
    return {
      key: row.id,
      kind: 'stack',
      title: row.label,
      desc: row.desc,
      control: (
        <Seg
          id={row.id}
          label={row.label}
          value={row.value}
          options={trOptions(row.options, t)}
          onChange={row.onChange}
        />
      ),
    };
  });
}

// 数据分区：字段名与 data-field 一一对应，只有存储路径行带额外 class。
function dataItems(doc, channel, t) {
  return [
    { key: 'channel', kind: 'row', label: t('settings_channel'), value: channel, field: 'channel' },
    {
      key: 'updated',
      kind: 'row',
      label: t('settings_updated'),
      value: doc.updated || '—',
      field: 'updated',
    },
    {
      key: 'samples',
      kind: 'row',
      label: t('settings_samples'),
      value: fmtInt(doc.samples_n),
      field: 'samples',
    },
    {
      key: 'sessions',
      kind: 'row',
      label: t('settings_sessions'),
      value: fmtInt(Array.isArray(doc.sessions) ? doc.sessions.length : null),
      field: 'sessions',
    },
    {
      key: 'retention',
      kind: 'row',
      label: t('settings_retention'),
      value: t('settings_retention_value'),
    },
    {
      key: 'storage',
      kind: 'row',
      label: t('settings_storage'),
      value: STORAGE_PATH,
      cls: 'set-value--path',
    },
  ];
}

// 子项：stack = 标题/说明 + 控件，row = 左标签 + 右值。
function SettingsItem({ item }) {
  if (item.kind === 'row') {
    return <SetRow label={item.label} value={item.value} field={item.field} cls={item.cls} />;
  }
  return <SetStack title={item.title} desc={item.desc} control={item.control} />;
}

// 设置页分区：标题 + 卡片；卡片子项间的 Divider 由 SetGroup 统一插入。
function SettingsSection({ title, items }) {
  return (
    <div class="settings-section">
      <div class="settings-title">{title}</div>
      <SetGroup>
        {items.map(function (item) {
          return <SettingsItem key={item.key} item={item} />;
        })}
      </SetGroup>
    </div>
  );
}

// 设置页：四段（外观/刷新/数据/关于）的 items 全部由数据描述，渲染只做一次映射。
function Settings(props) {
  const { t, lang } = useTr();
  const doc = props.doc || {};
  const channelKey = CHANNEL_KEY[doc.channel];
  const channel = channelKey ? t(channelKey) : doc.channel || '—';
  const sections = [
    {
      key: 'appearance',
      title: t('settings_appearance'),
      items: appearanceItems(props, t, lang),
    },
    {
      key: 'refresh',
      title: t('settings_refresh'),
      items: [
        {
          key: 'refresh_now',
          kind: 'stack',
          title: t('settings_manual_refresh'),
          desc: t('settings_refresh_desc'),
          control: (
            <button id="refresh" type="button" disabled={props.busy} onClick={props.onRefresh}>
              {props.busy ? t('refreshing') : t('refresh_now')}
            </button>
          ),
        },
      ],
    },
    {
      key: 'data',
      title: t('settings_data'),
      items: dataItems(doc, channel, t),
    },
    {
      key: 'about',
      title: t('settings_about'),
      items: [
        {
          key: 'about',
          kind: 'stack',
          title: 'ChargingRecord Evolution',
          desc: t('settings_about_desc'),
        },
        { key: 'webui', kind: 'row', label: t('settings_webui'), value: t('settings_webui_value') },
      ],
    },
  ];
  return (
    <section class={'page' + (props.active ? ' is-active' : '')} id="page_settings" role="tabpanel">
      {sections.map(function (section) {
        return <SettingsSection key={section.key} title={section.title} items={section.items} />;
      })}
    </section>
  );
}
export { Settings };
