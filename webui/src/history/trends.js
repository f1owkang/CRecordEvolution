import { plural } from '../i18n.js';
import { E } from '../ui/index.js';
import { CHART_H, CHART_W, fmtInt, fmtShortDate } from '../core.js';
import { pickPairs, svgBars, svgLine, svgScatter, svgTsLine } from '../charts.js';

function TrendTile(props) {
    var clickable = !!props.small;
    function onKey(e) {
      if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); if (clickable) props.onToggle(); }
    }
    return E('div', {
      class: 'trend-tile' + (props.open ? ' is-open' : ''),
      role: clickable ? 'button' : null,
      tabIndex: clickable ? 0 : null,
      'aria-expanded': clickable ? (props.open ? 'true' : 'false') : null,
      onClick: clickable ? props.onToggle : null,
      onKeyDown: clickable ? onKey : null
    },
      E('div', { class: 'trend-head' },
        E('span', { class: 'trend-title' }, props.title),
        E('span', { class: 'trend-value' }, props.value)),
      clickable
        ? E('div', { class: 'trend-spark', dangerouslySetInnerHTML: { __html: props.small } })
        : E('div', { class: 'trend-empty' }, props.empty || '—'),
      clickable && props.open && props.big
        ? E('div', { class: 'trend-big', dangerouslySetInnerHTML: { __html: props.big } })
        : null,
      clickable && props.open && props.detail ? E('div', { class: 'trend-det' }, props.detail) : null,
      props.cap ? E('div', { class: 'trend-cap' }, props.cap) : null);
  }

function trendsView(c, tr, openTile, setOpenTile) {
    var lang = c.lang;
    tr = tr || {};
    function toggleTile(key) {
      setOpenTile(openTile === key ? null : key);
    }
    var estRows = Array.isArray(tr.estimates) ? tr.estimates : [];
    var estPoints = pickPairs(estRows, 'ts', 'mah');
    var capSvg = estPoints.length ? svgLine(estRows, CHART_W, CHART_H, 'var(--accent)') : '';
    var latestEst = estRows.length ? estRows[estRows.length - 1] : null;
    var capVal = latestEst ? fmtInt(latestEst.mah) + ' mAh' : '—';

    var sessions = (Array.isArray(tr.sessions) ? tr.sessions : []).filter(function (s) { return s && s.valid; });
    var sesBig = sessions.length ? svgBars(sessions, CHART_W, CHART_H, 'var(--blue)') : '';
    var sesVal = sessions.length ? plural(lang, sessions.length, 'hist_point_one', 'hist_point_many', { n: sessions.length }, 'hist_point_zero') : '—';
    var latestSes = sessions.length ? sessions[sessions.length - 1] : null;
    if (latestSes && latestSes.delta_pct != null) {
      sesVal = c.t('hist_session_gain', { n: fmtInt(latestSes.delta_pct) });
    }
    var sesCap = plural(lang, sessions.length, 'hist_point_one', 'hist_point_many', { n: sessions.length }, 'hist_point_zero');
    if (sessions.length >= 2) {
      sesCap += ' · ' + fmtShortDate(lang, sessions[0].end_ts) + ' – ' + fmtShortDate(lang, sessions[sessions.length - 1].end_ts);
    }

    var disRows = Array.isArray(tr.discharge) ? tr.discharge : [];
    var disPoints = pickPairs(disRows, 'ts', 'uah_mah');
    var disSvg = disPoints.length ? svgTsLine(disRows, CHART_W, 64, 'var(--accent)', 'uah_mah') : '';
    var disBig = disPoints.length ? svgTsLine(disRows, CHART_W, CHART_H, 'var(--accent)', 'uah_mah') : '';
    var disSum = 0;
    disRows.forEach(function (r) { var v = Number(r && r.uah_mah); if (isFinite(v)) disSum += v; });
    var disVal = disRows.length ? fmtInt(Math.round(disSum)) + ' mAh' : '—';

    var restRows = Array.isArray(tr.rest_points) ? tr.rest_points : [];
    var restPoints = pickPairs(restRows, 'ts', 'uv');
    var restSvg = restPoints.length ? svgScatter(restRows, CHART_W, 64, 'var(--rest-dot)') : '';
    var restBig = restPoints.length ? svgScatter(restRows, CHART_W, CHART_H, 'var(--rest-dot)') : '';
    var latestRest = restRows.length ? restRows[restRows.length - 1] : null;
    var restVal = latestRest ? (Number(latestRest.uv) / 1e6).toFixed(3) + ' V' : '—';

    var ccctRows = Array.isArray(tr.ccct) ? tr.ccct : [];
    var ccctPoints = pickPairs(ccctRows, 'ts', 'secs');
    var ccctSvg = ccctPoints.length ? svgTsLine(ccctRows, CHART_W, 64, 'var(--purple)', 'secs') : '';
    var ccctBig = ccctPoints.length ? svgTsLine(ccctRows, CHART_W, CHART_H, 'var(--purple)', 'secs') : '';
    var latestCcct = ccctRows.length ? ccctRows[ccctRows.length - 1] : null;
    var ccctVal = latestCcct ? (Number(latestCcct.secs) / 60).toFixed(1) + ' min' : '—';

    var icaRows = (Array.isArray(tr.ica_peaks) ? tr.ica_peaks : []).filter(function (r) { return r && r.peak_h_rel != null; });
    var icaPoints = pickPairs(icaRows, 'ts', 'peak_h_rel');
    var icaSvg = icaPoints.length ? svgTsLine(icaRows, CHART_W, 64, 'var(--orange)', 'peak_h_rel', 1) : '';
    var icaBig = icaPoints.length ? svgTsLine(icaRows, CHART_W, CHART_H, 'var(--orange)', 'peak_h_rel', 1) : '';
    var latestIca = icaRows.length ? icaRows[icaRows.length - 1] : null;
    var icaVal = latestIca ? Number(latestIca.peak_h_rel).toFixed(2) : '—';

    var any = estPoints.length || sessions.length || disPoints.length || restPoints.length || ccctPoints.length || icaPoints.length;
    if (!any) {
      return E('div', { class: 'hist-empty' }, E('div', { class: 't' }, c.t('hist_empty_trends')));
    }

    var pointsCap = function (points) {
      var txt = plural(lang, points.length, 'hist_point_one', 'hist_point_many', { n: points.length }, 'hist_point_zero');
      if (points.length >= 2) {
        txt += ' · ' + fmtShortDate(lang, points[0].x) + ' – ' + fmtShortDate(lang, points[points.length - 1].x);
      }
      return txt;
    };

    return E('div', { class: 'trend-wrap' },
      E('section', { class: 'trend-hero' },
        E('div', { class: 'trend-head' },
          E('span', { class: 'trend-title' }, c.t('hist_trend_capacity')),
          E('span', { class: 'trend-value' }, capVal)),
        capSvg
          ? E('div', { class: 'chart', dangerouslySetInnerHTML: { __html: capSvg } })
          : E('div', { class: 'trend-empty' }, c.t('hist_empty_trends')),
        E('div', { class: 'trend-cap' }, pointsCap(estPoints)),
        latestEst && estPoints.length >= 2 ? E('div', { class: 'trend-cap' },
          c.t('hist_est_delta', { n: (latestEst.mah - estRows[estRows.length - 2].mah >= 0 ? '+' : '') + fmtInt(latestEst.mah - estRows[estRows.length - 2].mah) })) : null),
      E('section', { class: 'trend-hero' },
        E('div', { class: 'trend-head' },
          E('span', { class: 'trend-title' }, c.t('hist_trend_sessions')),
          E('span', { class: 'trend-value' }, sesVal)),
        sesBig
          ? E('div', { class: 'chart', dangerouslySetInnerHTML: { __html: sesBig } })
          : E('div', { class: 'trend-empty' }, c.t('hist_empty_records')),
        E('div', { class: 'trend-cap' }, sesCap)),
      E('div', { class: 'trend-grid' },
        E(TrendTile, {
          title: c.t('hist_trend_discharge'), value: disVal, small: disSvg, big: disBig,
          open: openTile === 'discharge', onToggle: function () { toggleTile('discharge'); },
          cap: pointsCap(disPoints),
          empty: c.t('hist_empty_records')
        }),
        E(TrendTile, {
          title: c.t('hist_trend_rest'), value: restVal, small: restSvg, big: restBig,
          open: openTile === 'rest', onToggle: function () { toggleTile('rest'); },
          cap: pointsCap(restPoints),
          empty: c.t('hist_empty_trends')
        }),
        E(TrendTile, {
          title: c.t('hist_trend_ccct'), value: ccctVal, small: ccctSvg, big: ccctBig,
          open: openTile === 'ccct', onToggle: function () { toggleTile('ccct'); },
          cap: pointsCap(ccctPoints),
          empty: c.t('hist_empty_trends')
        }),
        E(TrendTile, {
          title: c.t('hist_trend_ica'), value: icaVal, small: icaSvg, big: icaBig,
          open: openTile === 'ica', onToggle: function () { toggleTile('ica'); },
          cap: pointsCap(icaPoints),
          empty: c.t('hist_empty_trends')
        })));
  }
export { trendsView };
