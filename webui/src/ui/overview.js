import { useState, useEffect } from 'preact/hooks';
import { plural } from '../i18n.js';
import { CHART_W, CHART_H, fmtInt, fmtFix, fmtTs } from '../core.js';
import { ascByTs, pickPairs, svgLine } from '../charts.js';
import { E, useTr } from './context.js';

function ChartCard(props) {
  const c = useTr();
  const [idx, setIdx] = useState(null);
  useEffect(
    function () {
      setIdx(null);
    },
    [props.svg]
  );
  if (props.hidden) return null;
  const hasSvg = !!props.svg && props.points && props.points.length > 0;
  function onClick(e) {
    let t = e.target;
    while (t && t !== e.currentTarget && !(t.getAttribute && t.getAttribute('data-i') != null))
      t = t.parentNode;
    const attr = t && t.getAttribute ? t.getAttribute('data-i') : null;
    if (attr == null) return;
    const i = Number(attr);
    if (!isFinite(i) || i < 0 || i >= props.points.length) return;
    setIdx(i);
  }
  return E(
    'section',
    { class: 'panel' },
    E('div', { class: 'ptitle' }, props.title),
    hasSvg
      ? E('div', {
          class: 'chart',
          onClick: onClick,
          dangerouslySetInnerHTML: { __html: props.svg },
        })
      : E('div', { class: 'row empty' }, props.empty),
    hasSvg
      ? E(
          'div',
          { class: 'det' },
          idx == null
            ? E('span', { class: 'hint' }, c.t('tap_hint'))
            : props.format(props.points[idx].row)
        )
      : null,
    props.cap ? E('div', { class: 'cap' }, props.cap) : null
  );
}

function Cell(props) {
  return E(
    'div',
    {
      class: 'cell' + (props.pt ? ' pt' : ''),
      style: props.style,
      title: props.title == null ? null : props.title,
      onClick: props.onClick,
    },
    E('div', { class: 'label' }, props.label),
    E('div', { class: 'val' }, props.value),
    props.detail || null
  );
}

function Overview(props) {
  const c = useTr();
  const lang = c.lang;
  const d = props.doc;
  const [qualOpen, setQualOpen] = useState(false);

  const design = d ? d.design_mah : null;
  const est = d ? d.est_mah : null;
  const hasEst = est != null && design != null && Number(design) > 0;
  const pctText = d ? (hasEst ? String(Math.round((est / design) * 100)) : fmtInt(d.pct)) : '--';
  const badgeClass = 'badge ' + (hasEst ? 'meas' : 'kernel');
  const badgeText = hasEst ? c.t('badge_measured') : c.t('badge_kernel');
  const subText = !d
    ? c.t('loading')
    : hasEst
      ? est + ' / ' + design + ' mAh'
      : d.full_mah != null
        ? c.t('sub_full', { mah: d.full_mah })
        : '\u00a0';

  let estText;
  if (est != null) {
    const sg = Number(d.est_mah_sigma);
    const hasSigma = d.channel === 'ml' && d.est_mah_sigma != null && isFinite(sg);
    estText =
      lang === 'en' && hasSigma
        ? est + ' ±' + Math.round(sg) + ' mAh'
        : est + ' mAh' + (hasSigma ? ' ±' + Math.round(sg) + ' mAh' : '');
  } else {
    const nsamp = d && d.samples != null ? d.samples : 0;
    estText = plural(
      lang,
      nsamp,
      'est_learning_one',
      'est_learning_many',
      { n: fmtInt(nsamp) },
      'est_learning_zero'
    );
  }

  const tw = d ? Number(d.trend_mah_per_week) : NaN;
  const hasTw = d && d.trend_mah_per_week != null && isFinite(tw);
  let trendText;
  if (d && d.trend_state === 'stable') trendText = c.t('trend_stable');
  else if (d && d.trend_state === 'insufficient') {
    trendText =
      c.t('trend_insufficient') +
      (d.trend_span_day != null ? c.t('trend_days', { n: fmtInt(d.trend_span_day) }) : '');
  } else if (hasTw)
    trendText =
      tw < 0
        ? c.t('trend_down', { n: Math.abs(tw).toFixed(1) })
        : c.t('trend_up', { n: tw.toFixed(1) });
  else trendText = '—';

  const q = d ? d.quality : null;
  const qOk = q != null && q.ok != null;
  const qDetail = [];
  if (qOk) {
    ['ema', 'mid', 'dis', 'full'].forEach(function (k) {
      if (q[k + '_mah'] != null) qDetail.push(c.t('ch_' + k) + ' ' + q[k + '_mah'] + ' mAh');
    });
  }
  const qDetailTxt = qOk ? c.t('qual_detail', { list: qDetail.join(' / ') }) : '';
  const qText = qOk
    ? c.t('qual_summary', { state: q.ok ? c.t('qual_ok') : c.t('qual_bad'), n: q.spread_pct })
    : c.t('qual_insufficient');

  const recent = d && Array.isArray(d.recent) ? d.recent : [];
  const recentAsc = ascByTs(recent);
  const designOK = design != null && Number(design) > 0;
  const showEst = designOK || recent.length > 0;
  const estPoints = pickPairs(recentAsc, 'ts', 'mah');
  const latest = recent.length ? recent[0] : null;
  const estSvg = estPoints.length
    ? svgLine(recentAsc, CHART_W, CHART_H, 'var(--accent)', hasTw ? tw : null)
    : '';
  const capParts = [];
  if (latest != null) capParts.push(c.t('est_cap_latest', { mah: fmtInt(latest.mah) }));
  if (d && d.samples_n != null)
    capParts.push(
      plural(
        lang,
        d.samples_n,
        'samples_one',
        'samples_many',
        { n: fmtInt(d.samples_n) },
        'samples_zero'
      )
    );
  const estCap = capParts.join(' · ');

  return E(
    'section',
    { class: 'page' + (props.active ? ' is-active' : ''), id: 'page_overview', role: 'tabpanel' },
    E(
      'section',
      { class: 'hero' },
      E('span', { class: badgeClass }, badgeText),
      E('div', { class: 'hero-line' }, E('span', null, pctText), E('span', { class: 'unit' }, '%')),
      E('div', { class: 'sub' }, subText)
    ),
    E(
      'section',
      { class: 'grid' },
      E(Cell, { label: c.t('label_design'), value: fmtInt(design) }),
      E(Cell, { label: c.t('label_full'), value: fmtInt(d ? d.full_mah : null) }),
      E(Cell, { label: c.t('label_cycles'), value: fmtInt(d ? d.cycles : null) }),
      E(Cell, { label: c.t('label_est'), value: estText }),
      E(Cell, { label: c.t('label_cyceq'), value: fmtFix(d ? d.cycle_equiv : null, 2) }),
      E(Cell, {
        label: c.t('label_rmoh'),
        value: d && d.r_moh != null ? Number(d.r_moh).toFixed(1) + ' mΩ' : c.t('rmoh_collecting'),
      }),
      E(
        'div',
        { class: 'cell', style: { gridColumn: 'span 2' } },
        E('div', { class: 'label' }, c.t('label_trend')),
        E('div', { class: 'val' }, trendText)
      ),
      E(Cell, {
        label: c.t('label_qual'),
        value: qText,
        style: { gridColumn: 'span 2' },
        pt: qOk,
        title: qOk ? qDetailTxt : null,
        onClick: qOk
          ? function () {
              setQualOpen(!qualOpen);
            }
          : null,
        detail: qOk && qualOpen ? E('div', { class: 'det' }, qDetailTxt) : null,
      })
    ),
    E(ChartCard, {
      hidden: !showEst,
      title: c.t('panel_est'),
      svg: estSvg,
      points: estPoints,
      empty: c.t('est_empty'),
      cap: estCap,
      format: function (r) {
        return fmtTs(lang, r.ts) + ' · ' + fmtInt(r.mah) + ' mAh';
      },
    })
  );
}

export { Overview };
