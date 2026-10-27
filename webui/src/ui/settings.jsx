import { useTr } from './context.js';
import { Seg, SetStack, SetRow, Divider } from './components.jsx';
import { CHANNEL_KEY, fmtInt } from '../core.js';
function Settings(props) {
  const { t, lang } = useTr();
  const d = props.doc || {};
  const channelKey = CHANNEL_KEY[d.channel];
  const channel = channelKey ? t(channelKey) : d.channel || '—';
  const themeOptions = [
    {
      value: 'auto',
      label: t('theme_auto'),
    },
    {
      value: 'light',
      label: t('theme_light'),
    },
    {
      value: 'dark',
      label: t('theme_dark'),
    },
  ];
  const langOptions = [
    {
      value: 'zh',
      label: t('lang_zh'),
    },
    {
      value: 'en',
      label: t('lang_en'),
    },
  ];
  const barOptions = [
    {
      value: 'floating',
      label: t('bar_floating'),
    },
    {
      value: 'classic',
      label: t('bar_classic'),
    },
  ];
  return (
    <section class={'page' + (props.active ? ' is-active' : '')} id="page_settings" role="tabpanel">
      <div class="settings-section">
        <div class="settings-title">{t('settings_appearance')}</div>
        <div class="set-group">
          <SetStack
            title={t('settings_theme')}
            desc={t('settings_theme_desc')}
            control={
              <Seg
                id="theme_seg"
                label={t('settings_theme')}
                value={props.themeMode}
                options={themeOptions}
                onChange={props.onThemeChange}
              />
            }
          />
          <Divider />
          <SetStack
            title={t('settings_language')}
            desc={t('settings_language_desc')}
            control={
              <Seg
                id="lang_seg"
                label={t('settings_language')}
                value={lang}
                options={langOptions}
                onChange={props.onLangChange}
              />
            }
          />
          <Divider />
          <SetStack
            title={t('settings_bar_mode')}
            desc={t('settings_bar_mode_desc')}
            control={
              <Seg
                id="bar_mode_seg"
                label={t('settings_bar_mode')}
                value={props.barMode}
                options={barOptions}
                onChange={props.onBarChange}
              />
            }
          />
        </div>
      </div>
      <div class="settings-section">
        <div class="settings-title">{t('settings_refresh')}</div>
        <div class="set-group">
          <SetStack
            title={t('settings_manual_refresh')}
            desc={t('settings_refresh_desc')}
            control={
              <button id="refresh" type="button" disabled={props.busy} onClick={props.onRefresh}>
                {props.busy ? t('refreshing') : t('refresh_now')}
              </button>
            }
          />
        </div>
      </div>
      <div class="settings-section">
        <div class="settings-title">{t('settings_data')}</div>
        <div class="set-group">
          <SetRow label={t('settings_channel')} value={channel} field="channel" />
          <Divider />
          <SetRow label={t('settings_updated')} value={d.updated || '—'} field="updated" />
          <Divider />
          <SetRow label={t('settings_samples')} value={fmtInt(d.samples_n)} field="samples" />
          <Divider />
          <SetRow
            label={t('settings_sessions')}
            value={fmtInt(Array.isArray(d.sessions) ? d.sessions.length : null)}
            field="sessions"
          />
          <Divider />
          <SetRow label={t('settings_retention')} value={t('settings_retention_value')} />
          <Divider />
          <SetRow
            label={t('settings_storage')}
            value="/data/adb/modules/CRecordEvolution/data"
            cls="set-value--path"
          />
        </div>
      </div>
      <div class="settings-section">
        <div class="settings-title">{t('settings_about')}</div>
        <div class="set-group">
          <div class="set-item set-item--stack">
            <div class="set-text">
              <div class="set-title">{'ChargingRecord Evolution'}</div>
              <div class="set-desc">{t('settings_about_desc')}</div>
            </div>
          </div>
          <Divider />
          <SetRow label={t('settings_webui')} value={t('settings_webui_value')} />
        </div>
      </div>
    </section>
  );
}
export { Settings };
