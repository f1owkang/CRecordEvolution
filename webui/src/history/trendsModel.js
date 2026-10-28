import { CHART_H, CHART_W, fmtInt, fmtShortDate } from '../core.js';
import { plural } from '../i18n.js';
import { pickPairs, svgBars, svgLine, svgScatter, svgTsLine } from '../charts.js';

// 迷你图与大图成对生成：无数据点时都返回空串，TrendTile 据此判「不可点」。
function chartPair(points, draw) {
  return points.length ? { small: draw(64), big: draw(CHART_H) } : { small: '', big: '' };
}

// 「N 个点 · 起–止日期」：容量 hero 与小多图共用；sessions 需要先映射成 { x: end_ts }。
function pointsCaption(lang, points) {
  let text = plural(
    lang,
    points.length,
    'hist_point_one',
    'hist_point_many',
    { n: points.length },
    'hist_point_zero'
  );
  if (points.length >= 2) {
    text +=
      ' · ' +
      fmtShortDate(lang, points[0].x) +
      ' – ' +
      fmtShortDate(lang, points[points.length - 1].x);
  }
  return text;
}

// 趋势页的派生模型：纯函数，把 trends 文档收敛成两张 hero 卡的文案/SVG 与四张小图数据，
// 组件只负责排版。保持原实现的计算顺序与空值降级（缺失即显示占位符）。
function buildTrendModel(trends, t, lang) {
  const trendData = trends || {};

  // 容量：估算点连线 + 最新估算值 + 与上一次估算的差值。
  const estRows = Array.isArray(trendData.estimates) ? trendData.estimates : [];
  const estPoints = pickPairs(estRows, 'ts', 'mah');
  const capSvg = estPoints.length ? svgLine(estRows, CHART_W, CHART_H, 'var(--accent)') : '';
  const latestEst = estRows.length ? estRows[estRows.length - 1] : null;
  const capVal = latestEst ? fmtInt(latestEst.mah) + ' mAh' : '—';
  let estDelta = null;
  if (latestEst && estPoints.length >= 2) {
    const diff = latestEst.mah - estRows[estRows.length - 2].mah;
    estDelta = t('hist_est_delta', { n: (diff >= 0 ? '+' : '') + fmtInt(diff) });
  }

  // 充电会话：只取结算有效的会话，值优先展示最近一次的涨幅。
  const sessions = (Array.isArray(trendData.sessions) ? trendData.sessions : []).filter(
    (session) => session && session.valid
  );
  const sesBig = sessions.length ? svgBars(sessions, CHART_W, CHART_H, 'var(--blue)') : '';
  let sesVal = sessions.length
    ? plural(
        lang,
        sessions.length,
        'hist_point_one',
        'hist_point_many',
        { n: sessions.length },
        'hist_point_zero'
      )
    : '—';
  const latestSession = sessions.length ? sessions[sessions.length - 1] : null;
  if (latestSession) {
    if (latestSession.start_cap != null && latestSession.end_cap != null) {
      sesVal = t('hist_session_soc', {
        a: fmtInt(latestSession.start_cap),
        b: fmtInt(latestSession.end_cap),
      });
    } else if (latestSession.delta_pct != null) {
      sesVal = t('hist_session_gain', { n: fmtInt(latestSession.delta_pct) });
    }
  }
  const sesCap = pointsCaption(
    lang,
    sessions.map((session) => ({ x: session.end_ts }))
  );

  // 放电：柱状小图，值取区间内放电容量之和。
  const disRows = Array.isArray(trendData.discharge) ? trendData.discharge : [];
  const disPoints = pickPairs(disRows, 'ts', 'uah_mah');
  const disPair = chartPair(disPoints, (height) =>
    svgTsLine(disRows, CHART_W, height, 'var(--accent)', 'uah_mah')
  );
  let disSum = 0;
  disRows.forEach((row) => {
    const value = Number(row && row.uah_mah);
    if (isFinite(value)) disSum += value;
  });
  const disVal = disRows.length ? fmtInt(Math.round(disSum)) + ' mAh' : '—';

  // 静置电压：散点小图，最新一点换算成 V。
  const restRows = Array.isArray(trendData.rest_points) ? trendData.rest_points : [];
  const restPoints = pickPairs(restRows, 'ts', 'uv');
  const restPair = chartPair(restPoints, (height) =>
    svgScatter(restRows, CHART_W, height, 'var(--rest-dot)')
  );
  const latestRest = restRows.length ? restRows[restRows.length - 1] : null;
  const restVal = latestRest ? (Number(latestRest.uv) / 1e6).toFixed(3) + ' V' : '—';

  // CCCT：恒流充电时长，最新一点换算成 min。
  const ccctRows = Array.isArray(trendData.ccct) ? trendData.ccct : [];
  const ccctPoints = pickPairs(ccctRows, 'ts', 'secs');
  const ccctPair = chartPair(ccctPoints, (height) =>
    svgTsLine(ccctRows, CHART_W, height, 'var(--purple)', 'secs')
  );
  const latestCcct = ccctRows.length ? ccctRows[ccctRows.length - 1] : null;
  const ccctVal = latestCcct ? (Number(latestCcct.secs) / 60).toFixed(1) + ' min' : '—';

  // ICA：峰值相对高度，参考线固定在 1。
  const icaRows = (Array.isArray(trendData.ica_peaks) ? trendData.ica_peaks : []).filter(
    (row) => row && row.peak_h_rel != null
  );
  const icaPoints = pickPairs(icaRows, 'ts', 'peak_h_rel');
  const icaPair = chartPair(icaPoints, (height) =>
    svgTsLine(icaRows, CHART_W, height, 'var(--orange)', 'peak_h_rel', 1)
  );
  const latestIca = icaRows.length ? icaRows[icaRows.length - 1] : null;
  const icaVal = latestIca ? Number(latestIca.peak_h_rel).toFixed(2) : '—';

  const hasAnyData =
    estPoints.length ||
    sessions.length ||
    disPoints.length ||
    restPoints.length ||
    ccctPoints.length ||
    icaPoints.length;

  // 四张小多图同构：用一张表描述数据源与文案，渲染处只写一次。
  const tiles = [
    {
      key: 'discharge',
      title: t('hist_trend_discharge'),
      value: disVal,
      ...disPair,
      cap: pointsCaption(lang, disPoints),
      empty: t('hist_empty_records'),
    },
    {
      key: 'rest',
      title: t('hist_trend_rest'),
      value: restVal,
      ...restPair,
      cap: pointsCaption(lang, restPoints),
      empty: t('hist_empty_trends'),
    },
    {
      key: 'ccct',
      title: t('hist_trend_ccct'),
      value: ccctVal,
      ...ccctPair,
      cap: pointsCaption(lang, ccctPoints),
      empty: t('hist_empty_trends'),
    },
    {
      key: 'ica',
      title: t('hist_trend_ica'),
      value: icaVal,
      ...icaPair,
      cap: pointsCaption(lang, icaPoints),
      empty: t('hist_empty_trends'),
    },
  ];

  return {
    capSvg,
    capVal,
    capCaption: pointsCaption(lang, estPoints),
    estDelta,
    sesBig,
    sesVal,
    sesCap,
    sesNote: sessions.length ? t('hist_sessions_note') : null,
    tiles,
    hasAnyData,
  };
}

export { buildTrendModel };
