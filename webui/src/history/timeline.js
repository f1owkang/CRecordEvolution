import { E, useTr } from '../ui/index.js';
import { REASON_KEY, fmtClock, fmtInt, fmtRange, fmtTs } from '../core.js';

// 把一条 history 事件（charge / discharge）转换成时间轴一行的视图模型：
// { title, primary, status, statusOk, invalid, sub, detail }。
function toTimelineRow(c, ev) {
  const lang = c.lang;
  const ch = ev && ev.charge;
  const dis = ev && ev.discharge;
  let invalid = false;
  let title,
    primary,
    status,
    statusOk = false;
  const sub = [],
    detail = [];

  function addDetail(k, v) {
    if (v != null && v !== '') detail.push([k, v]);
  }

  if (ch) {
    title = c.t('panel_ses');
    primary =
      ch.delta_pct != null
        ? c.t('hist_session_gain', { n: fmtInt(ch.delta_pct) })
        : c.t('panel_ses');
    invalid = ch.valid === false;
    if (invalid) {
      status =
        ch.invalid_reason && REASON_KEY[ch.invalid_reason]
          ? c.t(REASON_KEY[ch.invalid_reason])
          : c.t('session_not_counted');
    } else {
      status = c.t('session_counted');
      statusOk = true;
    }
    if (ch.est_mah != null) sub.push(c.t('hist_session_est', { n: fmtInt(ch.est_mah) }));
    if (ch.temp_avg != null) sub.push(c.t('hist_session_temp', { n: fmtInt(ch.temp_avg) }));
    const mins = Math.round((Number(ch.end_ts) - Number(ch.start_ts)) / 60);
    if (isFinite(mins) && mins > 0) sub.push(c.t('hist_duration', { n: mins }));
    addDetail(c.t('hist_detail_time'), fmtRange(lang, ch.start_ts, ch.end_ts));
    if (ch.temp_avg != null) addDetail(c.t('hist_detail_temp'), fmtInt(ch.temp_avg) + ' ℃');
    if (ch.c_rate != null) addDetail(c.t('hist_detail_rate'), Number(ch.c_rate).toFixed(2) + ' C');
    if (ch.est_mah != null) addDetail(c.t('hist_detail_estimate'), fmtInt(ch.est_mah) + ' mAh');
  } else if (dis) {
    title = c.t('panel_dis');
    primary = fmtInt(dis.uah_mah) + ' mAh';
    if (dis.invalid_reason && REASON_KEY[dis.invalid_reason]) {
      status = c.t(REASON_KEY[dis.invalid_reason]);
      invalid = true;
    } else {
      status = c.t('hist_status_recorded');
      statusOk = true;
    }
    if (dis.start_cap != null && dis.end_cap != null)
      sub.push(fmtInt(dis.start_cap) + '→' + fmtInt(dis.end_cap) + '%');
    const dmins = Math.round(Number(dis.secs) / 60);
    if (isFinite(dmins)) sub.push(c.t('hist_duration', { n: dmins }));
    if (dis.implied_mah != null) sub.push(c.t('hist_implied', { n: fmtInt(dis.implied_mah) }));
    addDetail(c.t('hist_detail_time'), fmtTs(lang, dis.ts));
    addDetail(c.t('hist_detail_duration'), dmins + ' min');
    if (dis.start_cap != null && dis.end_cap != null)
      addDetail(c.t('hist_detail_soc'), fmtInt(dis.start_cap) + '→' + fmtInt(dis.end_cap) + '%');
    addDetail(c.t('hist_detail_released'), fmtInt(dis.uah_mah) + ' mAh');
    if (dis.implied_mah != null)
      addDetail(c.t('hist_detail_implied'), fmtInt(dis.implied_mah) + ' mAh');
  } else {
    return null;
  }

  return {
    title: title,
    primary: primary,
    status: status,
    statusOk: statusOk,
    invalid: invalid,
    sub: sub,
    detail: detail,
  };
}

// 时间轴一行：默认只显示标题/时间/主指标/两个副指标，点按展开详情。
function TimelineEvent(props) {
  const c = useTr();
  const info = toTimelineRow(c, props.event);
  if (!info) return null;
  const detailEls = [];
  if (props.open) {
    info.detail.forEach(function (row, i) {
      detailEls.push(E('div', { key: 'k' + i, class: 'k' }, row[0]));
      detailEls.push(E('div', { key: 'v' + i, class: 'v' }, row[1]));
    });
  }
  return E(
    'div',
    { class: 'tl-item ' + props.kind + (info.invalid ? ' is-invalid' : '') },
    E('div', { class: 'tl-rail' }, E('span', { class: 'tl-dot' })),
    E(
      'div',
      {
        class: 'tl-card' + (props.open ? ' is-open' : '') + (info.invalid ? ' is-invalid' : ''),
        role: 'button',
        tabIndex: 0,
        'aria-expanded': props.open ? 'true' : 'false',
        'aria-label': info.title + ' ' + info.primary,
        onClick: props.onToggle,
        onKeyDown: function (e) {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            props.onToggle();
          }
        },
      },
      E(
        'div',
        { class: 'tl-top' },
        E('span', { class: 'tl-kind' }, info.title),
        E('span', { class: 'tl-time' }, fmtClock(props.event.ts)),
        info.status
          ? E('span', { class: 'pill ' + (info.statusOk ? 'ok' : 'warn') }, info.status)
          : null,
        E('span', { class: 'tl-chev', 'aria-hidden': 'true' })
      ),
      E('div', { class: 'tl-value' }, info.primary),
      info.sub.length
        ? E(
            'div',
            { class: 'tl-sub' },
            info.sub.map(function (s, i) {
              return E('span', { key: i }, s);
            })
          )
        : null,
      props.open ? E('div', { class: 'tl-det' }, detailEls) : null
    )
  );
}

export { TimelineEvent };
