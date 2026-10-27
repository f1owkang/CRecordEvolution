import { useTr } from '../ui/index.js';
import { REASON_KEY, fmtClock, fmtInt, fmtRange, fmtTs } from '../core.js';

// 把一条 history 事件（charge / discharge）转换成时间轴一行的视图模型：
// { title, primary, status, statusOk, invalid, sub, detail }。
// t / lang 由调用方从 LangCtx 传入；event 为 history 接口返回的单条事件。
function toTimelineRow(event, t, lang) {
  const charge = event && event.charge;
  const discharge = event && event.discharge;
  let invalid = false;
  let title, primary, status;
  let statusOk = false;
  const sub = [],
    detail = [];

  function addDetail(label, value) {
    if (value != null && value !== '') detail.push([label, value]);
  }

  if (charge) {
    title = t('panel_ses');
    primary =
      charge.delta_pct != null
        ? t('hist_session_gain', { n: fmtInt(charge.delta_pct) })
        : t('panel_ses');
    invalid = charge.valid === false;
    if (invalid) {
      status =
        charge.invalid_reason && REASON_KEY[charge.invalid_reason]
          ? t(REASON_KEY[charge.invalid_reason])
          : t('session_not_counted');
    } else {
      status = t('session_counted');
      statusOk = true;
    }
    if (charge.est_mah != null) sub.push(t('hist_session_est', { n: fmtInt(charge.est_mah) }));
    if (charge.temp_avg != null) sub.push(t('hist_session_temp', { n: fmtInt(charge.temp_avg) }));
    const minutes = Math.round((Number(charge.end_ts) - Number(charge.start_ts)) / 60);
    if (isFinite(minutes) && minutes > 0) sub.push(t('hist_duration', { n: minutes }));
    addDetail(t('hist_detail_time'), fmtRange(lang, charge.start_ts, charge.end_ts));
    if (charge.temp_avg != null) addDetail(t('hist_detail_temp'), fmtInt(charge.temp_avg) + ' ℃');
    if (charge.c_rate != null)
      addDetail(t('hist_detail_rate'), Number(charge.c_rate).toFixed(2) + ' C');
    if (charge.est_mah != null)
      addDetail(t('hist_detail_estimate'), fmtInt(charge.est_mah) + ' mAh');
  } else if (discharge) {
    title = t('panel_dis');
    primary = fmtInt(discharge.uah_mah) + ' mAh';
    if (discharge.invalid_reason && REASON_KEY[discharge.invalid_reason]) {
      status = t(REASON_KEY[discharge.invalid_reason]);
      invalid = true;
    } else {
      status = t('hist_status_recorded');
      statusOk = true;
    }
    if (discharge.start_cap != null && discharge.end_cap != null)
      sub.push(fmtInt(discharge.start_cap) + '→' + fmtInt(discharge.end_cap) + '%');
    const dischargeMinutes = Math.round(Number(discharge.secs) / 60);
    if (isFinite(dischargeMinutes)) sub.push(t('hist_duration', { n: dischargeMinutes }));
    if (discharge.implied_mah != null)
      sub.push(t('hist_implied', { n: fmtInt(discharge.implied_mah) }));
    addDetail(t('hist_detail_time'), fmtTs(lang, discharge.ts));
    addDetail(t('hist_detail_duration'), dischargeMinutes + ' min');
    if (discharge.start_cap != null && discharge.end_cap != null)
      addDetail(
        t('hist_detail_soc'),
        fmtInt(discharge.start_cap) + '→' + fmtInt(discharge.end_cap) + '%'
      );
    addDetail(t('hist_detail_released'), fmtInt(discharge.uah_mah) + ' mAh');
    if (discharge.implied_mah != null)
      addDetail(t('hist_detail_implied'), fmtInt(discharge.implied_mah) + ' mAh');
  } else {
    return null;
  }

  return { title, primary, status, statusOk, invalid, sub, detail };
}

// 时间轴一行：默认只显示标题/时间/主指标/两个副指标，点按展开详情。
function TimelineEvent({ event, kind, open, onToggle }) {
  const { t, lang } = useTr();
  const row = toTimelineRow(event, t, lang);
  if (!row) return null;

  // 详情按「标签 / 值」两列平铺，key 由下标与列标记组成（行内容稳定）。
  const detailEls = [];
  if (open) {
    row.detail.forEach((detailRow, index) => {
      detailEls.push(
        <div key={'k' + index} class="k">
          {detailRow[0]}
        </div>
      );
      detailEls.push(
        <div key={'v' + index} class="v">
          {detailRow[1]}
        </div>
      );
    });
  }

  return (
    <div class={'tl-item ' + kind + (row.invalid ? ' is-invalid' : '')}>
      <div class="tl-rail">
        <span class="tl-dot" />
      </div>
      <div
        class={'tl-card' + (open ? ' is-open' : '') + (row.invalid ? ' is-invalid' : '')}
        role="button"
        tabIndex={0}
        aria-expanded={open ? 'true' : 'false'}
        aria-label={row.title + ' ' + row.primary}
        onClick={onToggle}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            onToggle();
          }
        }}
      >
        <div class="tl-top">
          <span class="tl-kind">{row.title}</span>
          <span class="tl-time">{fmtClock(event.ts)}</span>
          {row.status ? (
            <span class={'pill ' + (row.statusOk ? 'ok' : 'warn')}>{row.status}</span>
          ) : null}
          <span class="tl-chev" aria-hidden="true" />
        </div>
        <div class="tl-value">{row.primary}</div>
        {row.sub.length ? (
          <div class="tl-sub">
            {row.sub.map((text, index) => (
              <span key={index}>{text}</span>
            ))}
          </div>
        ) : null}
        {open ? <div class="tl-det">{detailEls}</div> : null}
      </div>
    </div>
  );
}

export { TimelineEvent, toTimelineRow };
