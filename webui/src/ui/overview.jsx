import { useState, useEffect } from 'preact/hooks';
import { fmtInt, fmtTs } from '../core.js';
import { tapIndex } from '../chartTap.js';
import { useTr } from './context.js';
import { buildOverviewModel } from './overviewModel.js';

function ChartCard(props) {
  const { t } = useTr();
  const [idx, setIdx] = useState(null);
  const points = props.points || [];
  // 只有「数据窗口本身」变了才清空选中（点数变化或最后一个点的时间戳变化）；
  // 不要依赖整个 doc 引用——温度每变 0.1℃ 都会换一份 doc，那样选中的点会被
  // 下一轮轮询清掉。
  const lastTs = points.length ? points[points.length - 1].row.ts : 0;
  useEffect(
    function () {
      setIdx(null);
    },
    [points.length, lastTs]
  );
  if (props.hidden) return null;
  const hasSvg = typeof props.svg === 'function' && points.length > 0;
  const selected = idx == null || idx >= points.length ? null : idx;
  return (
    <section class="panel">
      <div class="ptitle">{props.title}</div>
      {hasSvg ? (
        <div
          class="chart"
          onClick={function (e) {
            setIdx(tapIndex(e, points.length));
          }}
          dangerouslySetInnerHTML={{
            __html: props.svg(selected),
          }}
        />
      ) : (
        <div class="row empty">{props.empty}</div>
      )}
      {hasSvg ? (
        <div class="det">
          {selected == null ? (
            <span class="hint">{t('tap_hint')}</span>
          ) : (
            props.format(points[selected].row)
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
      role={props.role || null}
      tabIndex={props.tabIndex == null ? null : props.tabIndex}
      aria-expanded={props.expanded == null ? null : props.expanded ? 'true' : 'false'}
      onClick={props.onClick}
      onKeyDown={props.onKeyDown}
    >
      <div class="label">{props.label}</div>
      <div class="val">{props.value}</div>
      {props.detail || null}
    </div>
  );
}

// 充电中实时卡：插在健康度主卡与指标格之间；model 为 null（无 live / 非充电）时整卡不渲染。
// 各分段（状态/指标/会话/预计）缺失时各自省略，占位符不进入 DOM。
function ChargeCard(props) {
  const model = props.model;
  if (!model) return null;
  return (
    <section class="charge-card" data-field="live">
      <div class="charge-head">
        <span class="charge-title">{model.title}</span>
        {model.statusText ? (
          <span class={'badge ' + model.statusClass}>{model.statusText}</span>
        ) : null}
      </div>
      {model.metrics.length ? (
        <div class="charge-metrics">
          {model.metrics.map(function (metric) {
            return (
              <div key={metric.key} class="charge-metric">
                <div class="label">{metric.label}</div>
                <div class="val">{metric.value}</div>
              </div>
            );
          })}
        </div>
      ) : null}
      {model.metaText ? <div class="charge-meta">{model.metaText}</div> : null}
    </section>
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
        {m.tempText ? <div class="hero-temp">{m.tempText}</div> : null}
      </section>
      <ChargeCard model={m.charge} />
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
          {m.lifeText ? <div class="det">{m.lifeText}</div> : null}
        </div>
        <Cell
          label={t('label_qual')}
          value={m.qualityText}
          style={{
            gridColumn: 'span 2',
          }}
          pt={m.qualityOk}
          title={m.qualityOk ? m.qualityDetailText : null}
          role={m.qualityOk ? 'button' : null}
          tabIndex={m.qualityOk ? 0 : null}
          expanded={m.qualityOk ? qualOpen : null}
          onClick={
            m.qualityOk
              ? function () {
                  setQualOpen(!qualOpen);
                }
              : null
          }
          onKeyDown={
            m.qualityOk
              ? function (e) {
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    setQualOpen(!qualOpen);
                  }
                }
              : null
          }
          detail={
            m.qualityOk ? (
              qualOpen ? (
                <div class="det">{m.qualityDetailText}</div>
              ) : (
                <div class="det">
                  <span class="hint">{t('qual_tap_hint')}</span>
                </div>
              )
            ) : null
          }
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
