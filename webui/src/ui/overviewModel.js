import { plural } from '../i18n.js';
import { CHART_W, CHART_H, fmtInt, fmtFix } from '../core.js';
import { ascByTs, pickPairs, svgLine } from '../charts.js';

// 质量哨兵的四路容量口径，数组顺序即详情里的展示顺序
const QUAL_CHANNELS = ['ema', 'mid', 'dis', 'full'];

// 主卡副标题：优先「估算 / 设计」，其次内核满充容量，最后用一个不换行空格占位
function overviewSubText(doc, hasEst, est, design, t) {
  if (!doc) return t('loading');
  if (hasEst) return est + ' / ' + design + ' mAh';
  if (doc.full_mah != null) return t('sub_full', { mah: doc.full_mah });
  return '\u00a0';
}

// 实测估算格：有估算值时展示容量（ML 通道且 sigma 有效时追加「±sigma」，
// 英文把 ± 放到容量前面），没有估算值时回退到学习进度文案
function overviewEstText(doc, lang, est, t) {
  if (est != null) {
    const sg = Number(doc.est_mah_sigma);
    const hasSigma = doc.channel === 'ml' && doc.est_mah_sigma != null && isFinite(sg);
    if (lang === 'en' && hasSigma) return est + ' ±' + Math.round(sg) + ' mAh';
    return est + ' mAh' + (hasSigma ? ' ±' + Math.round(sg) + ' mAh' : '');
  }
  const nsamp = doc && doc.samples != null ? doc.samples : 0;
  return plural(
    lang,
    nsamp,
    'est_learning_one',
    'est_learning_many',
    { n: fmtInt(nsamp) },
    'est_learning_zero'
  );
}

// 容量趋势格：stable / insufficient 优先，其次按周斜率走回升 / 衰减文案
function overviewTrendText(doc, hasTw, tw, t) {
  if (doc && doc.trend_state === 'stable') return t('trend_stable');
  if (doc && doc.trend_state === 'insufficient') {
    return (
      t('trend_insufficient') +
      (doc.trend_span_day != null ? t('trend_days', { n: fmtInt(doc.trend_span_day) }) : '')
    );
  }
  if (!hasTw) return '—';
  if (tw < 0) return t('trend_down', { n: fmtFix(Math.abs(tw), 1) });
  return t('trend_up', { n: fmtFix(tw, 1) });
}

// 可信度详情：只列本机已有的容量口径，按固定顺序用「 / 」拼接
function overviewQualDetail(q, t) {
  return QUAL_CHANNELS.filter(function (k) {
    return q[k + '_mah'] != null;
  }).map(function (k) {
    return t('ch_' + k) + ' ' + q[k + '_mah'] + ' mAh';
  });
}

// 概览页的派生模型：纯函数，只把 JSON 文档变成可渲染的字符串 / 布尔 / 点集，
// 不调 hook、不碰 DOM，便于单测与逐字符比对。
function buildOverviewModel(doc, lang, t) {
  const d = doc;
  const design = d ? d.design_mah : null;
  const est = d ? d.est_mah : null;
  const hasEst = est != null && design != null && Number(design) > 0;
  const pctText = d ? (hasEst ? String(Math.round((est / design) * 100)) : fmtInt(d.pct)) : '--';
  const badgeClass = 'badge ' + (hasEst ? 'meas' : 'kernel');
  const badgeText = hasEst ? t('badge_measured') : t('badge_kernel');
  const subText = overviewSubText(d, hasEst, est, design, t);
  const estText = overviewEstText(d, lang, est, t);
  const tw = d ? Number(d.trend_mah_per_week) : NaN;
  const hasTw = d && d.trend_mah_per_week != null && isFinite(tw);
  const trendText = overviewTrendText(d, hasTw, tw, t);
  const q = d ? d.quality : null;
  const qOk = q != null && q.ok != null;
  const qDetailTxt = qOk ? t('qual_detail', { list: overviewQualDetail(q, t).join(' / ') }) : '';
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
  if (latest != null) capParts.push(t('est_cap_latest', { mah: fmtInt(latest.mah) }));
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
  return {
    badgeClass,
    badgeText,
    pctText,
    subText,
    designText: fmtInt(design),
    fullText: fmtInt(d ? d.full_mah : null),
    cyclesText: fmtInt(d ? d.cycles : null),
    estText,
    cyceqText: fmtFix(d ? d.cycle_equiv : null, 2),
    rmohText: d && d.r_moh != null ? fmtFix(d.r_moh, 1) + ' mΩ' : t('rmoh_collecting'),
    trendText,
    qOk,
    qText,
    qDetailTxt,
    showEst,
    estSvg,
    estPoints,
    estCap,
  };
}

export { buildOverviewModel };
