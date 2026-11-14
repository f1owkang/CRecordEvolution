import { plural } from '../i18n.js';
import { CHART_W, CHART_H, chargeActive, fmtGroup, fmtInt, fmtFix } from '../core.js';
import { ascByTs, pickPairs, svgLine } from '../charts.js';

// 质量哨兵的四路容量口径，数组顺序即详情里的展示顺序
const QUAL_CHANNELS = ['ema', 'mid', 'dis', 'full'];

// 数值字段可能缺失/为 null：只有有限数才参与展示，其余返回 null 由调用方省略。
function finiteNumber(v) {
  if (v == null) return null;
  const n = Number(v);
  return isFinite(n) ? n : null;
}

// 充电状态：Charging/Full/Discharging 之外（含未知字符串）统一「未充电」；
// 只有 charging 会话、没有 status 的旧数据按充电中处理（否则卡片没有状态可显示）。
function chargeStatusText(live, t) {
  if (live.status === 'Charging') return t('charge_status_charging');
  if (live.status === 'Full') return t('charge_status_full');
  if (live.status === 'Discharging') return t('charge_status_discharging');
  if (live.status == null && live.charging) return t('charge_status_charging');
  if (live.status != null) return t('charge_status_idle');
  return null;
}

// 实时电流/功率/电压：单位换算只在展示层，数字与单位之间留空格。
function chargeMetric(t, labelKey, value) {
  return value == null ? null : { key: labelKey, label: t(labelKey), value };
}

function chargeCurrent(ua) {
  const n = finiteNumber(ua);
  return n == null ? null : fmtFix(n / 1e6, 2) + ' A';
}

function chargePower(mw) {
  const n = finiteNumber(mw);
  return n == null ? null : fmtFix(n / 1000, 1) + ' W';
}

function chargeVoltage(uv) {
  const n = finiteNumber(uv);
  return n == null ? null : fmtFix(n / 1e6, 2) + ' V';
}

// 本次会话：61% → 76% · 已充 1,234 mAh · 12 分钟；cap 缺失时回退 live.cap，
// start_cap/acc_uah/secs 各自缺失时只省略对应分段，不输出占位符。
function chargeSessionText(live, t) {
  const c = live.charging;
  if (!c || typeof c !== 'object') return null;
  const parts = [];
  const startCap = finiteNumber(c.start_cap);
  const endCap = finiteNumber(c.cap != null ? c.cap : live.cap);
  if (startCap != null && endCap != null) {
    parts.push(t('charge_soc', { a: fmtInt(startCap), b: fmtInt(endCap) }));
  }
  const accUah = finiteNumber(c.acc_uah);
  if (accUah != null) parts.push(t('charge_added', { mah: fmtGroup(accUah / 1000) }));
  const secs = finiteNumber(c.secs);
  if (secs != null && secs >= 0) {
    parts.push(t('charge_duration', { n: fmtInt(Math.round(secs / 60)) }));
  }
  return parts.length ? parts.join(' · ') : null;
}

// 预计充满：<60 分钟按分钟，≥60 分钟拆成「x 小时 y 分钟」，整点小时省略分钟。
function chargeEtaText(live, t) {
  const c = live.charging;
  const n = c && typeof c === 'object' ? finiteNumber(c.eta_min) : null;
  if (n == null || n < 0) return null;
  const min = Math.round(n);
  if (min < 60) return t('charge_eta_min', { n: fmtInt(min) });
  const h = Math.floor(min / 60);
  const m = min % 60;
  return m
    ? t('charge_eta_hm', { h: fmtInt(h), m: fmtInt(m) })
    : t('charge_eta_h', { h: fmtInt(h) });
}

// 概览「充电中实时卡」：显隐判定与全部文案都在这层算好，组件只铺 DOM。
// 旧 daemon 没有 live 或非充电态时整卡返回 null；lang 目前不参与格式化，
// 保留与 buildOverviewModel 一致的签名，便于按语言分流。
function buildChargeModel(doc, lang, t) {
  if (!chargeActive(doc)) return null;
  const live = doc.live;
  const metrics = [
    chargeMetric(t, 'charge_current', chargeCurrent(live.current_ua)),
    chargeMetric(t, 'charge_power', chargePower(live.power_mw)),
    chargeMetric(t, 'charge_voltage', chargeVoltage(live.voltage_uv)),
  ].filter(function (item) {
    return item != null;
  });
  return {
    title: t('charge_title'),
    statusText: chargeStatusText(live, t),
    statusClass: live.status === 'Charging' || !live.status ? 'meas' : 'kernel',
    metrics,
    // 会话进度与预计充满合并成一行；任一段缺失时只保留其余段落。
    metaText:
      [chargeSessionText(live, t), chargeEtaText(live, t)].filter(Boolean).join(' · ') || null,
  };
}

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

// 终点预测：后端只在趋势显著、方向为衰减且未跌破 80% 时下发 weeks_to_80。
// 前端只做单位换算（<9 周按周，其余按月，1 个月≈4.345 周），不拿斜率外推；
// 月份最小为 2，只有周数可能为 1，故周文案走 plural。
function overviewLifeText(doc, lang, t) {
  const weeks = doc && doc.weeks_to_80 != null ? Number(doc.weeks_to_80) : NaN;
  if (!isFinite(weeks) || weeks < 1) return null;
  if (weeks < 9) {
    return plural(lang, weeks, 'life_weeks_one', 'life_weeks', { n: fmtInt(weeks) });
  }
  const months = Math.max(2, Math.round(weeks / 4.345));
  return t('life_months', { n: fmtInt(months) });
}

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
  const lifeText = overviewLifeText(doc, lang, t);
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
  // 温度 meta 行：temp_c 缺失时整行省略（不显示占位符）；数值与 ℃ 之间留空格。
  const tempC = doc ? Number(doc.temp_c) : NaN;
  const tempText =
    doc != null && doc.temp_c != null && isFinite(tempC)
      ? t('label_temp') + ' ' + fmtFix(tempC, 1) + ' ℃'
      : null;
  const recent = doc && Array.isArray(doc.recent) ? doc.recent : [];
  const recentAsc = ascByTs(recent);
  const designOK = design != null && Number(design) > 0;
  const showEst = designOK || recent.length > 0;
  const estPoints = pickPairs(recentAsc, 'ts', 'mah');
  const latest = recent.length ? recent[0] : null;
  // 选中点高亮要重画 SVG：estSvg 返回一个按选中下标生成图的函数，
  // 组件只负责把当前下标传进来（图表数据仍全部来自这里）。
  const estSvg = function (sel) {
    return estPoints.length
      ? svgLine(recentAsc, CHART_W, CHART_H, 'var(--accent)', trendLine, sel)
      : '';
  };
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
    lifeText,
    qualityOk,
    qualityText,
    qualityDetailText,
    showEst,
    tempText,
    estSvg,
    estPoints,
    estCap,
    charge: buildChargeModel(doc, lang, t),
  };
}

export { buildOverviewModel, buildChargeModel };
