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
    const sigmaMah = Number(doc.est_mah_sigma);
    const hasSigma = doc.channel === 'ml' && doc.est_mah_sigma != null && isFinite(sigmaMah);
    if (lang === 'en' && hasSigma) return est + ' ±' + Math.round(sigmaMah) + ' mAh';
    return est + ' mAh' + (hasSigma ? ' ±' + Math.round(sigmaMah) + ' mAh' : '');
  }
  const sampleCount = doc && doc.samples != null ? doc.samples : 0;
  return plural(
    lang,
    sampleCount,
    'est_learning_one',
    'est_learning_many',
    { n: fmtInt(sampleCount) },
    'est_learning_zero'
  );
}

// 容量趋势格：stable / insufficient 优先，其次按周斜率走回升 / 衰减文案
function overviewTrendText(doc, hasTrendMahPerWeek, trendMahPerWeek, t) {
  if (doc && doc.trend_state === 'stable') return t('trend_stable');
  if (doc && doc.trend_state === 'insufficient') {
    return (
      t('trend_insufficient') +
      (doc.trend_span_day != null ? t('trend_days', { n: fmtInt(doc.trend_span_day) }) : '')
    );
  }
  if (!hasTrendMahPerWeek) return '—';
  if (trendMahPerWeek < 0) return t('trend_down', { n: fmtFix(Math.abs(trendMahPerWeek), 1) });
  return t('trend_up', { n: fmtFix(trendMahPerWeek, 1) });
}

// 可信度详情：只列本机已有的容量口径，按固定顺序用「 / 」拼接
function overviewQualDetail(quality, t) {
  return QUAL_CHANNELS.filter(function (channelKey) {
    return quality[channelKey + '_mah'] != null;
  }).map(function (channelKey) {
    return t('ch_' + channelKey) + ' ' + quality[channelKey + '_mah'] + ' mAh';
  });
}

// 概览页的派生模型：纯函数，只把 JSON 文档变成可渲染的字符串 / 布尔 / 点集，
// 不调 hook、不碰 DOM，便于单测与逐字符比对。
function buildOverviewModel(doc, lang, t) {
  const design = doc ? doc.design_mah : null;
  const est = doc ? doc.est_mah : null;
  const hasEst = est != null && design != null && Number(design) > 0;
  const pctText = doc
    ? hasEst
      ? String(Math.round((est / design) * 100))
      : fmtInt(doc.pct)
    : '--';
  const badgeClass = 'badge ' + (hasEst ? 'meas' : 'kernel');
  const badgeText = hasEst ? t('badge_measured') : t('badge_kernel');
  const subText = overviewSubText(doc, hasEst, est, design, t);
  const estText = overviewEstText(doc, lang, est, t);
  const trendMahPerWeek = doc ? Number(doc.trend_mah_per_week) : NaN;
  const hasTrendMahPerWeek = doc && doc.trend_mah_per_week != null && isFinite(trendMahPerWeek);
  const trendText = overviewTrendText(doc, hasTrendMahPerWeek, trendMahPerWeek, t);
  // 趋势线端点由后端给出（与 recent 同一份 estimates 拟合）；旧 JSON 没有这两个
  // 字段时不画虚线，避免前端拿首点+斜率外推导致线整体偏移。
  const trendLine =
    doc && doc.trend_from_mah != null && doc.trend_to_mah != null
      ? { from: Number(doc.trend_from_mah), to: Number(doc.trend_to_mah) }
      : null;
  const quality = doc ? doc.quality : null;
  const qualityOk = quality != null && quality.ok != null;
  const qualityDetailText = qualityOk
    ? t('qual_detail', { list: overviewQualDetail(quality, t).join(' / ') })
    : '';
  const qualityText = qualityOk
    ? t('qual_summary', {
        state: quality.ok ? t('qual_ok') : t('qual_bad'),
        n: quality.spread_pct,
      })
    : t('qual_insufficient');
  const recent = doc && Array.isArray(doc.recent) ? doc.recent : [];
  const recentAsc = ascByTs(recent);
  const designOK = design != null && Number(design) > 0;
  const showEst = designOK || recent.length > 0;
  const estPoints = pickPairs(recentAsc, 'ts', 'mah');
  const latest = recent.length ? recent[0] : null;
  const estSvg = estPoints.length
    ? svgLine(recentAsc, CHART_W, CHART_H, 'var(--accent)', trendLine)
    : '';
  const capParts = [];
  if (latest != null) capParts.push(t('est_cap_latest', { mah: fmtInt(latest.mah) }));
  if (doc && doc.samples_n != null)
    capParts.push(
      plural(
        lang,
        doc.samples_n,
        'samples_one',
        'samples_many',
        { n: fmtInt(doc.samples_n) },
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
    fullText: fmtInt(doc ? doc.full_mah : null),
    cyclesText: fmtInt(doc ? doc.cycles : null),
    estText,
    cyceqText: fmtFix(doc ? doc.cycle_equiv : null, 2),
    rmohText: doc && doc.r_moh != null ? fmtFix(doc.r_moh, 1) + ' mΩ' : t('rmoh_collecting'),
    trendText,
    qualityOk,
    qualityText,
    qualityDetailText,
    showEst,
    estSvg,
    estPoints,
    estCap,
  };
}

export { buildOverviewModel };
