import { useState } from 'preact/hooks';
import { useTr } from './context.js';
import { Seg, SetGroup, SetStack, SetRow } from './components.jsx';
import { CHANNEL_KEY, CMD_DIAG, copyText, fmtInt, run } from '../core.js';

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
// 电流单位缩写是固定符号；auto 表示没有 ×1/×1000 的量纲初判或刻度非整幂次。
const CALIB_UNIT_LABEL = { ua: 'µA', ma: 'mA' };

// Seg 的 options 只存 i18n key，渲染时再翻译，避免在模块级捕获 t。
function trOptions(options, t) {
  return options.map(function (option) {
    return { value: option.value, label: t(option.key) };
  });
}

// 电流刻度单位：ua/ma 是量纲符号，auto（或缺失）回落到“量纲未知”文案。
function calibUnitLabel(t, unit) {
  return CALIB_UNIT_LABEL[unit] || t('calib_unit_auto');
}

// 校准诊断行：Go 侧未下发 calib 时整段省略。
function calibItems(doc, t) {
  const calib = doc.calib;
  if (!calib || typeof calib !== 'object') return [];
  const unit = calibUnitLabel(t, calib.current_unit);
  // 刻度数值（×1000 还是 ×1）有值时一并展示
  const scaling =
    calib.current_scale == null
      ? t('calib_auto', { unit })
      : t('calib_calibrated', { scale: calib.current_scale, unit });
  return [
    {
      key: 'current_node',
      kind: 'row',
      label: t('settings_current_node'),
      value: calib.current_node || '—',
      field: 'current_node',
      cls: 'set-value--path',
    },
    {
      key: 'current_calib',
      kind: 'row',
      label: t('settings_current_calib'),
      value: scaling,
      field: 'current_calib',
    },
  ];
}

// 诊断复制：按钮 + 结果提示；剪贴板失败时回退成可选中的只读文本框（长按复制）。
function diagItem(t, diag, onCopy) {
  const control = (
    <div class="diag-box">
      <button id="diag" type="button" disabled={diag.busy} onClick={onCopy}>
        {diag.busy ? t('diag_busy') : t('diag_copy')}
      </button>
      {diag.message ? (
        <div id="diag-status" class={'diag-msg' + (diag.error ? ' is-err' : '')} role="status">
          {diag.message}
        </div>
      ) : null}
      {diag.text ? <textarea id="diag-text" class="diag-text" readOnly value={diag.text} /> : null}
    </div>
  );
  return {
    key: 'diag',
    kind: 'stack',
    title: t('settings_diag'),
    desc: t('settings_diag_desc'),
    control,
  };
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

// 数据分区：字段名与 data-field 一一对应，只有路径类行带额外 class。
function dataItems(doc, channel, t) {
  const rows = [
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
      // sessions 数组只是最近 10 条，数量必须用后端 sessions_n；旧 daemon 没有
      // 该字段时才回退数组长度。
      value: fmtInt(
        doc.sessions_n != null
          ? doc.sessions_n
          : Array.isArray(doc.sessions)
            ? doc.sessions.length
            : null
      ),
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
  return rows.concat(calibItems(doc, t));
}

// 关于页品牌卡：图标 + 名称 + 一句话说明 + 通道/版本胶囊。
function AboutHero(props) {
  return (
    <div class="about-hero">
      <span class="about-mark" aria-hidden="true" />
      <div class="about-name">ChargingRecord Evolution</div>
      <div class="about-tagline">{props.tagline}</div>
      <div class="about-chips">
        {props.chips.map(function (chip) {
          return (
            <span key={chip.label} class={'about-chip' + (chip.accent ? ' is-accent' : '')}>
              {chip.label}
            </span>
          );
        })}
      </div>
    </div>
  );
}

// 子项：stack = 标题/说明 + 控件，row = 左标签 + 右值，hero = 关于页品牌卡。
function SettingsItem({ item }) {
  if (item.kind === 'hero') {
    return <AboutHero tagline={item.tagline} chips={item.chips} />;
  }
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

// 复制诊断：Go 侧生成脱敏短文本 → 剪贴板；失败时把文本放进只读 textarea 兜底。
function useDiagCopy(t) {
  const [diag, setDiag] = useState({ busy: false, message: '', text: '', error: false });
  function copyDiag() {
    if (diag.busy) return;
    setDiag({ busy: true, message: '', text: '', error: false });
    run(CMD_DIAG).then(function (r) {
      const out = String(r.stdout || '').trim();
      if (Number(r.errno) !== 0 || !out) {
        setDiag({ busy: false, message: t('diag_error'), text: '', error: true });
        return;
      }
      copyText(out).then(function (ok) {
        setDiag(
          ok
            ? { busy: false, message: t('diag_copied'), text: '', error: false }
            : { busy: false, message: t('diag_copy_fail'), text: out, error: true }
        );
      });
    });
  }
  return { diag, copyDiag };
}

// 「刷新」段：页内手动刷新按钮（设置页顶栏不显示刷新按钮）。
function refreshSection(props, t) {
  return {
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
  };
}

// 「关于」段：品牌卡（通道 + 版本胶囊）+ 版本 / WebUI / 网络 / 许可 / 第三方信息行。
function aboutSection(doc, channel, t) {
  const version = doc.version || '';
  const chips = [{ label: channel, accent: true }];
  if (version) chips.push({ label: 'v' + version });
  return {
    key: 'about',
    title: t('settings_about'),
    items: [
      { key: 'about_hero', kind: 'hero', tagline: t('settings_about_desc'), chips: chips },
      {
        key: 'version',
        kind: 'row',
        label: t('about_version'),
        value: version || '—',
        field: 'version',
      },
      { key: 'webui', kind: 'row', label: t('settings_webui'), value: t('settings_webui_value') },
      { key: 'network', kind: 'row', label: t('about_network'), value: t('about_network_value') },
      { key: 'license', kind: 'row', label: t('about_license'), value: t('about_license_value') },
      {
        key: 'third_party',
        kind: 'row',
        label: t('about_third_party'),
        value: t('about_third_party_value'),
      },
    ],
  };
}

// 设置页：四段（外观/刷新/数据/关于）的 items 全部由数据描述，渲染只做一次映射。
function Settings(props) {
  const { t, lang } = useTr();
  const doc = props.doc || {};
  const channelKey = CHANNEL_KEY[doc.channel];
  const channel = channelKey ? t(channelKey) : doc.channel || '—';
  const { diag, copyDiag } = useDiagCopy(t);
  const dataRows = dataItems(doc, channel, t);
  dataRows.push(diagItem(t, diag, copyDiag));
  const sections = [
    { key: 'appearance', title: t('settings_appearance'), items: appearanceItems(props, t, lang) },
    refreshSection(props, t),
    { key: 'data', title: t('settings_data'), items: dataRows },
    aboutSection(doc, channel, t),
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
