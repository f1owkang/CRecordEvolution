import { useTr } from '../ui/index.js';
import { plural } from '../i18n.js';
import { CHART_H, CHART_W, fmtInt, fmtShortDate } from '../core.js';
import { pickPairs, svgBars, svgLine, svgScatter, svgTsLine } from '../charts.js';

// 小多图卡片：标题 + 最新值 + 迷你图；只有 small 存在时才是可点开大图的入口。
function TrendTile({ title, value, small, big, open, onToggle, cap, empty }) {
  const clickable = !!small;
  function onKey(e) {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      if (clickable) onToggle();
    }
  }
  return (
    <div
      class={'trend-tile' + (open ? ' is-open' : '')}
      role={clickable ? 'button' : null}
      tabIndex={clickable ? 0 : null}
      aria-expanded={clickable ? (open ? 'true' : 'false') : null}
      onClick={clickable ? onToggle : null}
      onKeyDown={clickable ? onKey : null}
    >
      <div class="trend-head">
        <span class="trend-title">{title}</span>
        <span class="trend-value">{value}</span>
      </div>
      {clickable ? (
        <div class="trend-spark" dangerouslySetInnerHTML={{ __html: small }} />
      ) : (
        <div class="trend-empty">{empty || '—'}</div>
      )}
      {clickable && open && big ? (
        <div class="trend-big" dangerouslySetInnerHTML={{ __html: big }} />
      ) : null}
      {cap ? <div class="trend-cap">{cap}</div> : null}
    </div>
  );
}

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

function TrendsView({ trends, openTile, onToggleTile }) {
  const { t, lang } = useTr();
  const trendData = trends || {};

  const estRows = Array.isArray(trendData.estimates) ? trendData.estimates : [];
  const estPoints = pickPairs(estRows, 'ts', 'mah');
  const capSvg = estPoints.length ? svgLine(estRows, CHART_W, CHART_H, 'var(--accent)') : '';
  const latestEst = estRows.length ? estRows[estRows.length - 1] : null;
  const capVal = latestEst ? fmtInt(latestEst.mah) + ' mAh' : '—';

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
  if (latestSession && latestSession.delta_pct != null) {
    sesVal = t('hist_session_gain', { n: fmtInt(latestSession.delta_pct) });
  }
  const sesCap = pointsCaption(
    lang,
    sessions.map((session) => ({ x: session.end_ts }))
  );

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

  const restRows = Array.isArray(trendData.rest_points) ? trendData.rest_points : [];
  const restPoints = pickPairs(restRows, 'ts', 'uv');
  const restPair = chartPair(restPoints, (height) =>
    svgScatter(restRows, CHART_W, height, 'var(--rest-dot)')
  );
  const latestRest = restRows.length ? restRows[restRows.length - 1] : null;
  const restVal = latestRest ? (Number(latestRest.uv) / 1e6).toFixed(3) + ' V' : '—';

  const ccctRows = Array.isArray(trendData.ccct) ? trendData.ccct : [];
  const ccctPoints = pickPairs(ccctRows, 'ts', 'secs');
  const ccctPair = chartPair(ccctPoints, (height) =>
    svgTsLine(ccctRows, CHART_W, height, 'var(--purple)', 'secs')
  );
  const latestCcct = ccctRows.length ? ccctRows[ccctRows.length - 1] : null;
  const ccctVal = latestCcct ? (Number(latestCcct.secs) / 60).toFixed(1) + ' min' : '—';

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
  if (!hasAnyData) {
    return (
      <div class="hist-empty">
        <div class="t">{t('hist_empty_trends')}</div>
      </div>
    );
  }

  // 四张小多图同构：用一张表描述数据源与文案，渲染处只写一次。
  const tiles = [
    {
      key: 'discharge',
      title: t('hist_trend_discharge'),
      value: disVal,
      points: disPoints,
      pair: disPair,
      empty: t('hist_empty_records'),
    },
    {
      key: 'rest',
      title: t('hist_trend_rest'),
      value: restVal,
      points: restPoints,
      pair: restPair,
      empty: t('hist_empty_trends'),
    },
    {
      key: 'ccct',
      title: t('hist_trend_ccct'),
      value: ccctVal,
      points: ccctPoints,
      pair: ccctPair,
      empty: t('hist_empty_trends'),
    },
    {
      key: 'ica',
      title: t('hist_trend_ica'),
      value: icaVal,
      points: icaPoints,
      pair: icaPair,
      empty: t('hist_empty_trends'),
    },
  ];

  return (
    <div class="trend-wrap">
      <section class="trend-hero">
        <div class="trend-head">
          <span class="trend-title">{t('hist_trend_capacity')}</span>
          <span class="trend-value">{capVal}</span>
        </div>
        {capSvg ? (
          <div class="chart" dangerouslySetInnerHTML={{ __html: capSvg }} />
        ) : (
          <div class="trend-empty">{t('hist_empty_trends')}</div>
        )}
        <div class="trend-cap">{pointsCaption(lang, estPoints)}</div>
        {latestEst && estPoints.length >= 2 ? (
          <div class="trend-cap">
            {t('hist_est_delta', {
              n:
                (latestEst.mah - estRows[estRows.length - 2].mah >= 0 ? '+' : '') +
                fmtInt(latestEst.mah - estRows[estRows.length - 2].mah),
            })}
          </div>
        ) : null}
      </section>

      <section class="trend-hero">
        <div class="trend-head">
          <span class="trend-title">{t('hist_trend_sessions')}</span>
          <span class="trend-value">{sesVal}</span>
        </div>
        {sesBig ? (
          <div class="chart" dangerouslySetInnerHTML={{ __html: sesBig }} />
        ) : (
          <div class="trend-empty">{t('hist_empty_records')}</div>
        )}
        <div class="trend-cap">{sesCap}</div>
      </section>

      <div class="trend-grid">
        {tiles.map((tile) => (
          <TrendTile
            key={tile.key}
            title={tile.title}
            value={tile.value}
            small={tile.pair.small}
            big={tile.pair.big}
            open={openTile === tile.key}
            onToggle={() => onToggleTile(openTile === tile.key ? null : tile.key)}
            cap={pointsCaption(lang, tile.points)}
            empty={tile.empty}
          />
        ))}
      </div>
    </div>
  );
}

export { TrendsView };
