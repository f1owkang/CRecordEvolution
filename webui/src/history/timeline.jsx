import { useTr } from '../ui/index.js';
import { REASON_KEY, fmtClock, fmtInt, fmtRange, fmtTs } from '../core.js';

// 记录里的 SOC 用「0~100% 轴上的区间条」表示：充放电都只占其中一段，既看得出
// 本次移动了多少个百分点，也看得出发生在电池的哪一段（顶部 80~100% 的读数本来
// 就更不可信）。条本身不带刻度，数值由上方「电量 52% → 88%」文字承担——图形是
// 文本的冗余编码，既满足不靠颜色/位置传达信息的要求，也避免窄区间标签互相挤压。
function clampPct(value) {
  return Math.max(0, Math.min(100, value));
}

function socRange(fromCap, toCap) {
  if (fromCap == null || toCap == null) return null;
  const from = Number(fromCap);
  const to = Number(toCap);
  if (!isFinite(from) || !isFinite(to)) return null;
  const cf = clampPct(from);
  const ct = clampPct(to);
  return { from: cf, to: ct, lo: Math.min(cf, ct), hi: Math.max(cf, ct) };
}

// 把一条 history 事件（charge / discharge）转换成时间轴一行的视图模型：
// { title, primary, status, statusOk, invalid, sub, detail, soc, socText }。
// t / lang 由调用方从 LangCtx 传入；event 为 history 接口返回的单条事件。
function toTimelineRow(event, t, lang) {
  const charge = event && event.charge;
  const discharge = event && event.discharge;
  let invalid = false;
  let title, primary, status, socText;
  let soc = null;
  let statusOk = false;
  const sub = [],
    detail = [];

  function addDetail(label, value) {
    if (value != null && value !== '') detail.push([label, value]);
  }

  if (charge) {
    title = t('panel_ses');
    primary =
      charge.start_cap != null && charge.end_cap != null
        ? t('hist_session_soc', {
            a: fmtInt(charge.start_cap),
            b: fmtInt(charge.end_cap),
          })
        : charge.delta_pct != null
          ? t('hist_session_gain', { n: fmtInt(charge.delta_pct) })
          : t('panel_ses');
    if (charge.start_cap != null && charge.end_cap != null) {
      soc = socRange(charge.start_cap, charge.end_cap);
      socText = t('hist_session_soc', { a: fmtInt(charge.start_cap), b: fmtInt(charge.end_cap) });
    }
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
    if (discharge.start_cap != null && discharge.end_cap != null) {
      soc = socRange(discharge.start_cap, discharge.end_cap);
      socText = t('hist_session_soc', {
        a: fmtInt(discharge.start_cap),
        b: fmtInt(discharge.end_cap),
      });
    }
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

  return { title, primary, status, statusOk, invalid, sub, detail, soc, socText };
}

// SOC 区间条：淡色「当前电量」0~to% + 本次变化段。充电用实心绿（新增电量），
// 放电用空心橙（已消耗，不再存在于当前电量里），所以两条不只靠颜色区分，也能
// 一眼看出方向。条对读屏隐藏，起止值由文字与卡片 aria-label 承担。
function SocBar({ soc, kind }) {
  const width = Math.max(soc.hi - soc.lo, 0) + '%';
  return (
    <div class="tl-soc" aria-hidden="true">
      <div class="tl-soc-track">
        <span class="tl-soc-level" style={{ width: soc.to + '%' }} />
        {kind === 'charge' ? (
          <span class="tl-soc-add" style={{ left: soc.lo + '%', width: width }} />
        ) : (
          <span class="tl-soc-lost" style={{ left: soc.lo + '%', width: width }} />
        )}
      </div>
    </div>
  );
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
        aria-label={
          row.socText && row.socText !== row.primary
            ? row.title + ' ' + row.primary + ' ' + row.socText
            : row.title + ' ' + row.primary
        }
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
        {row.soc ? <SocBar soc={row.soc} kind={kind} /> : null}
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
