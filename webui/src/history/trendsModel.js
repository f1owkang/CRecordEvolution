import { CHART_H, CHART_W, fmtInt, fmtShortDate, fmtTs } from '../core.js';
import { plural } from '../i18n.js';
import { ascByTs, pickPairs, svgBars, svgLine, svgScatter, svgTsLine } from '../charts.js';

// 迷你图与大图成对生成：无数据点时都返回空串，TrendTile 据此判「不可点」。
// draw(height, sel) 由各图决定画法；sel 是选中下标（小图永远不传）。
function chartPair(points, draw, sel) {
  if (!points.length) return { small: '', big: '' };
  return { small: draw(64, null), big: draw(CHART_H, sel) };
}

// 升序数据行 + 有效值过滤：图表内部按 ts 升序绘制，点按用的 data-i 必须与
// 同一份过滤后的行数组对齐，否则会读成相邻的点。
function ascRows(rows, key) {
  return pickPairs(ascByTs(rows), 'ts', key).map(function (pair) {
    return pair.row;
  });
}

// 点按读数统一为「时间 · 数值」，单位只在展示层拼。
function timeValue(lang, ts, value) {
  return fmtTs(lang, ts) + ' · ' + value;
}

function capacityReadout(lang, row) {
  return timeValue(lang, row.ts, fmtInt(row.mah) + ' mAh');
}

function sessionReadout(lang, t, row) {
  const parts = [fmtTs(lang, row.end_ts)];
  if (row.start_cap != null && row.end_cap != null) {
    parts.push(t('hist_session_soc', { a: fmtInt(row.start_cap), b: fmtInt(row.end_cap) }));
  } else if (row.delta_pct != null) {
    parts.push(t('hist_session_gain', { n: fmtInt(row.delta_pct) }));
  }
  if (row.est_mah != null) parts.push(t('hist_session_est', { n: fmtInt(row.est_mah) }));
  return parts.join(' · ');
}

function dischargeReadout(lang, row) {
  return timeValue(lang, row.ts, fmtInt(row.uah_mah) + ' mAh');
}

function restReadout(lang, row) {
  return timeValue(lang, row.ts, (Number(row.uv) / 1e6).toFixed(3) + ' V');
}

function ccctReadout(lang, row) {
  return timeValue(lang, row.ts, (Number(row.secs) / 60).toFixed(1) + ' min');
}

function icaReadout(lang, row) {
  return timeValue(lang, row.ts, Number(row.peak_h_rel).toFixed(2));
}

// 小多图/大图的选中下标；没选中时返回 null，由 charts 忽略。
function tileSel(sel, key) {
  return sel && sel.tiles ? sel.tiles[key] : null;
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

// 趋势页的派生模型：纯函数，把 trends 文档收敛成两张 hero 卡与小多图的
// 文案/SVG/点按读数，组件只负责排版。第 4 个参数 sel = { cap, ses, tiles }
// 传入各图的选中下标（用于画高亮圈），越界下标由 charts 内部忽略。
function buildTrendModel(trends, t, lang, sel) {
  const trendData = trends || {};
  sel = sel || {};

  // 容量：估算点连线 + 最新估算值 + 与上一次估算的差值。
  const estRows = Array.isArray(trendData.estimates) ? trendData.estimates : [];
  const capRows = ascRows(estRows, 'mah');
  const capSvg = capRows.length
    ? svgLine(capRows, CHART_W, CHART_H, 'var(--accent)', null, sel.cap)
    : '';
  const latestEst = capRows.length ? capRows[capRows.length - 1] : null;
  const capVal = latestEst ? fmtInt(latestEst.mah) + ' mAh' : '—';
  let estDelta = null;
  if (latestEst && capRows.length >= 2) {
    const diff = latestEst.mah - capRows[capRows.length - 2].mah;
    estDelta = t('hist_est_delta', { n: (diff >= 0 ? '+' : '') + fmtInt(diff) });
  }

  // 充电会话：只取结算有效、且有涨幅的会话，值优先展示最近一次的涨幅。
  const sessions = (Array.isArray(trendData.sessions) ? trendData.sessions : []).filter(
    (session) => session && session.valid && isFinite(Number(session.delta_pct))
  );
  // svgBars 按传入顺序画柱，data-i 即数组下标：这里显式按结束时间升序，
  // 与图表从左到右的时间轴一致。
  const sesRows = sessions.slice().sort(function (a, b) {
    return Number(a.end_ts) - Number(b.end_ts);
  });
  const sesBig = sesRows.length ? svgBars(sesRows, CHART_W, CHART_H, 'var(--blue)', sel.ses) : '';
  let sesVal = sesRows.length
    ? plural(
        lang,
        sesRows.length,
        'hist_point_one',
        'hist_point_many',
        { n: sesRows.length },
        'hist_point_zero'
      )
    : '—';
  const latestSession = sesRows.length ? sesRows[sesRows.length - 1] : null;
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
    sesRows.map((session) => ({ x: session.end_ts }))
  );

  // 放电：折线小图，值取区间内放电容量之和。
  const disRows = Array.isArray(trendData.discharge) ? trendData.discharge : [];
  const disPoints = pickPairs(ascByTs(disRows), 'ts', 'uah_mah');
  const disAsc = disPoints.map(function (pair) {
    return pair.row;
  });
  const disPair = chartPair(
    disPoints,
    function (height, selected) {
      return svgTsLine(disAsc, CHART_W, height, 'var(--accent)', 'uah_mah', null, selected);
    },
    tileSel(sel, 'discharge')
  );
  let disSum = 0;
  disAsc.forEach((row) => {
    const value = Number(row && row.uah_mah);
    if (isFinite(value)) disSum += value;
  });
  const disVal = disAsc.length ? fmtInt(Math.round(disSum)) + ' mAh' : '—';

  // 静置电压：散点小图，最新一点换算成 V。
  const restRows = Array.isArray(trendData.rest_points) ? trendData.rest_points : [];
  const restPoints = pickPairs(ascByTs(restRows), 'ts', 'uv');
  const restAsc = restPoints.map(function (pair) {
    return pair.row;
  });
  const restPair = chartPair(
    restPoints,
    function (height, selected) {
      return svgScatter(restAsc, CHART_W, height, 'var(--rest-dot)', selected);
    },
    tileSel(sel, 'rest')
  );
  const latestRest = restAsc.length ? restAsc[restAsc.length - 1] : null;
  const restVal = latestRest ? (Number(latestRest.uv) / 1e6).toFixed(3) + ' V' : '—';

  // CCCT：恒流充电时长，最新一点换算成 min。
  const ccctRows = Array.isArray(trendData.ccct) ? trendData.ccct : [];
  const ccctPoints = pickPairs(ascByTs(ccctRows), 'ts', 'secs');
  const ccctAsc = ccctPoints.map(function (pair) {
    return pair.row;
  });
  const ccctPair = chartPair(
    ccctPoints,
    function (height, selected) {
      return svgTsLine(ccctAsc, CHART_W, height, 'var(--purple)', 'secs', null, selected);
    },
    tileSel(sel, 'ccct')
  );
  const latestCcct = ccctAsc.length ? ccctAsc[ccctAsc.length - 1] : null;
  const ccctVal = latestCcct ? (Number(latestCcct.secs) / 60).toFixed(1) + ' min' : '—';

  // ICA：峰值相对高度，参考线固定在 1。
  const icaRows = (Array.isArray(trendData.ica_peaks) ? trendData.ica_peaks : []).filter(
    (row) => row && row.peak_h_rel != null
  );
  const icaPoints = pickPairs(ascByTs(icaRows), 'ts', 'peak_h_rel');
  const icaAsc = icaPoints.map(function (pair) {
    return pair.row;
  });
  const icaPair = chartPair(
    icaPoints,
    function (height, selected) {
      return svgTsLine(icaAsc, CHART_W, height, 'var(--orange)', 'peak_h_rel', 1, selected);
    },
    tileSel(sel, 'ica')
  );
  const latestIca = icaAsc.length ? icaAsc[icaAsc.length - 1] : null;
  const icaVal = latestIca ? Number(latestIca.peak_h_rel).toFixed(2) : '—';

  const hasAnyData =
    capRows.length ||
    sesRows.length ||
    disAsc.length ||
    restAsc.length ||
    ccctAsc.length ||
    icaAsc.length;

  // 四张小多图同构：用一张表描述数据源、文案与点按读数格式，渲染处只写一次。
  const tiles = [
    {
      key: 'discharge',
      title: t('hist_trend_discharge'),
      value: disVal,
      ...disPair,
      rows: disAsc,
      format: function (row) {
        return dischargeReadout(lang, row);
      },
      cap: pointsCaption(lang, disPoints),
      empty: t('hist_empty_records'),
    },
    {
      key: 'rest',
      title: t('hist_trend_rest'),
      value: restVal,
      ...restPair,
      rows: restAsc,
      format: function (row) {
        return restReadout(lang, row);
      },
      cap: pointsCaption(lang, restPoints),
      empty: t('hist_empty_trends'),
    },
    {
      key: 'ccct',
      title: t('hist_trend_ccct'),
      value: ccctVal,
      ...ccctPair,
      rows: ccctAsc,
      format: function (row) {
        return ccctReadout(lang, row);
      },
      cap: pointsCaption(lang, ccctPoints),
      empty: t('hist_empty_trends'),
    },
    {
      key: 'ica',
      title: t('hist_trend_ica'),
      value: icaVal,
      ...icaPair,
      rows: icaAsc,
      format: function (row) {
        return icaReadout(lang, row);
      },
      cap: pointsCaption(lang, icaPoints),
      empty: t('hist_empty_trends'),
    },
  ];

  return {
    capSvg,
    capVal,
    capCaption: pointsCaption(lang, pickPairs(capRows, 'ts', 'mah')),
    capRows,
    capFormat: function (row) {
      return capacityReadout(lang, row);
    },
    estDelta,
    sesBig,
    sesVal,
    sesCap,
    sesNote: sesRows.length ? t('hist_sessions_note') : null,
    sesRows,
    sesFormat: function (row) {
      return sessionReadout(lang, t, row);
    },
    tiles,
    hasAnyData,
  };
}

export { buildTrendModel };
