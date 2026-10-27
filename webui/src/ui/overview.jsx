import { useState, useEffect } from 'preact/hooks';
import { fmtInt, fmtTs } from '../core.js';
import { useTr } from './context.js';
import { buildOverviewModel } from './overviewModel.js';
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
  const [qualOpen, setQualOpen] = useState(false);
  const m = buildOverviewModel(props.doc, lang, t);
  return (
    <section class={'page' + (props.active ? ' is-active' : '')} id="page_overview" role="tabpanel">
      <section class="hero">
        <span class={m.badgeClass}>{m.badgeText}</span>
        <div class="hero-line">
          <span>{m.pctText}</span>
          <span class="unit">{'%'}</span>
        </div>
        <div class="sub">{m.subText}</div>
      </section>
      <section class="grid">
        <Cell label={t('label_design')} value={m.designText} />
        <Cell label={t('label_full')} value={m.fullText} />
        <Cell label={t('label_cycles')} value={m.cyclesText} />
        <Cell label={t('label_est')} value={m.estText} />
        <Cell label={t('label_cyceq')} value={m.cyceqText} />
        <Cell label={t('label_rmoh')} value={m.rmohText} />
        <div
          class="cell"
          style={{
            gridColumn: 'span 2',
          }}
        >
          <div class="label">{t('label_trend')}</div>
          <div class="val">{m.trendText}</div>
        </div>
        <Cell
          label={t('label_qual')}
          value={m.qText}
          style={{
            gridColumn: 'span 2',
          }}
          pt={m.qOk}
          title={m.qOk ? m.qDetailTxt : null}
          onClick={
            m.qOk
              ? function () {
                  setQualOpen(!qualOpen);
                }
              : null
          }
          detail={m.qOk && qualOpen ? <div class="det">{m.qDetailTxt}</div> : null}
        />
      </section>
      <ChartCard
        hidden={!m.showEst}
        title={t('panel_est')}
        svg={m.estSvg}
        points={m.estPoints}
        empty={t('est_empty')}
        cap={m.estCap}
        format={function (r) {
          return fmtTs(lang, r.ts) + ' · ' + fmtInt(r.mah) + ' mAh';
        }}
      />
    </section>
  );
}
export { Overview };
