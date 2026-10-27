import { useState, useEffect } from 'preact/hooks';
import { plural } from '../i18n.js';
import { CHART_W, CHART_H, fmtInt, fmtFix, fmtTs } from '../core.js';
import { ascByTs, pickPairs, svgLine } from '../charts.js';
import { useTr } from './context.js';
function ChartCard(props) {
  const { t, lang } = useTr();
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
  return (
    <section class="panel">
      <div class="ptitle">{props.title}</div>
      {hasSvg ? (
        <div
          class="chart"
          onClick={onClick}
          dangerouslySetInnerHTML={{
            __html: props.svg,
          }}
        />
      ) : (
        <div class="row empty">{props.empty}</div>
      )}
      {hasSvg ? (
        <div class="det">
          {idx == null ? (
            <span class="hint">{t('tap_hint')}</span>
          ) : (
            props.format(props.points[idx].row)
          )}
        </div>
      ) : null}
      {props.cap ? <div class="cap">{props.cap}</div> : null}
    </section>
  );
}
function Cell(props) {
  return (
    <div
      class={'cell' + (props.pt ? ' pt' : '')}
      style={props.style}
      title={props.title == null ? null : props.title}
      onClick={props.onClick}
    >
      <div class="label">{props.label}</div>
      <div class="val">{props.value}</div>
      {props.detail || null}
    </div>
  );
}
function Overview(props) {
  const { t, lang } = useTr();
  const d = props.doc;
  const [qualOpen, setQualOpen] = useState(false);
  const design = d ? d.design_mah : null;
  const est = d ? d.est_mah : null;
  const hasEst = est != null && design != null && Number(design) > 0;
  const pctText = d ? (hasEst ? String(Math.round((est / design) * 100)) : fmtInt(d.pct)) : '--';
  const badgeClass = 'badge ' + (hasEst ? 'meas' : 'kernel');
  const badgeText = hasEst ? t('badge_measured') : t('badge_kernel');
  const subText = !d
    ? t('loading')
    : hasEst
      ? est + ' / ' + design + ' mAh'
      : d.full_mah != null
        ? t('sub_full', {
            mah: d.full_mah,
          })
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
      {
        n: fmtInt(nsamp),
      },
      'est_learning_zero'
    );
  }
  const tw = d ? Number(d.trend_mah_per_week) : NaN;
  const hasTw = d && d.trend_mah_per_week != null && isFinite(tw);
  let trendText;
  if (d && d.trend_state === 'stable') trendText = t('trend_stable');
  else if (d && d.trend_state === 'insufficient') {
    trendText =
      t('trend_insufficient') +
      (d.trend_span_day != null
        ? t('trend_days', {
            n: fmtInt(d.trend_span_day),
          })
        : '');
  } else if (hasTw)
    trendText =
      tw < 0
        ? t('trend_down', {
            n: Math.abs(tw).toFixed(1),
          })
        : t('trend_up', {
            n: tw.toFixed(1),
          });
  else trendText = '—';
  const q = d ? d.quality : null;
  const qOk = q != null && q.ok != null;
  const qDetail = [];
  if (qOk) {
    ['ema', 'mid', 'dis', 'full'].forEach(function (k) {
      if (q[k + '_mah'] != null) qDetail.push(t('ch_' + k) + ' ' + q[k + '_mah'] + ' mAh');
    });
  }
  const qDetailTxt = qOk
    ? t('qual_detail', {
        list: qDetail.join(' / '),
      })
    : '';
  const qText = qOk
    ? t('qual_summary', {
        state: q.ok ? t('qual_ok') : t('qual_bad'),
        n: q.spread_pct,
      })
    : t('qual_insufficient');
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
  if (latest != null)
    capParts.push(
      t('est_cap_latest', {
        mah: fmtInt(latest.mah),
      })
    );
  if (d && d.samples_n != null)
    capParts.push(
      plural(
        lang,
        d.samples_n,
        'samples_one',
        'samples_many',
        {
          n: fmtInt(d.samples_n),
        },
        'samples_zero'
      )
    );
  const estCap = capParts.join(' · ');
  return (
    <section class={'page' + (props.active ? ' is-active' : '')} id="page_overview" role="tabpanel">
      <section class="hero">
        <span class={badgeClass}>{badgeText}</span>
        <div class="hero-line">
          <span>{pctText}</span>
          <span class="unit">{'%'}</span>
        </div>
        <div class="sub">{subText}</div>
      </section>
      <section class="grid">
        <Cell label={t('label_design')} value={fmtInt(design)} />
        <Cell label={t('label_full')} value={fmtInt(d ? d.full_mah : null)} />
        <Cell label={t('label_cycles')} value={fmtInt(d ? d.cycles : null)} />
        <Cell label={t('label_est')} value={estText} />
        <Cell label={t('label_cyceq')} value={fmtFix(d ? d.cycle_equiv : null, 2)} />
        <Cell
          label={t('label_rmoh')}
          value={d && d.r_moh != null ? Number(d.r_moh).toFixed(1) + ' mΩ' : t('rmoh_collecting')}
        />
        <div
          class="cell"
          style={{
            gridColumn: 'span 2',
          }}
        >
          <div class="label">{t('label_trend')}</div>
          <div class="val">{trendText}</div>
        </div>
        <Cell
          label={t('label_qual')}
          value={qText}
          style={{
            gridColumn: 'span 2',
          }}
          pt={qOk}
          title={qOk ? qDetailTxt : null}
          onClick={
            qOk
              ? function () {
                  setQualOpen(!qualOpen);
                }
              : null
          }
          detail={qOk && qualOpen ? <div class="det">{qDetailTxt}</div> : null}
        />
      </section>
      <ChartCard
        hidden={!showEst}
        title={t('panel_est')}
        svg={estSvg}
        points={estPoints}
        empty={t('est_empty')}
        cap={estCap}
        format={function (r) {
          return fmtTs(lang, r.ts) + ' · ' + fmtInt(r.mah) + ' mAh';
        }}
      />
    </section>
  );
}
export { Overview };
