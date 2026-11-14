import { describe, expect, it } from 'vitest';
import { translate } from '../i18n.js';
import { buildOverviewModel } from './overviewModel.js';

const zh = (key, vars) => translate('zh', key, vars);
const en = (key, vars) => translate('en', key, vars);

// 概览 JSON 的最小夹具：只保留模型读取的字段，其余由用例覆盖。
function doc(extra) {
  return {
    channel: 'stable',
    design_mah: 5000,
    est_mah: 4600,
    full_mah: 4800,
    cycles: 300,
    pct: 96,
    samples: 12,
    cycle_equiv: 120.5,
    r_moh: 35.2,
    temp_c: 31.5,
    samples_n: 4200,
    recent: [{ ts: 1700000000, mah: 4600 }],
    ...extra,
  };
}

describe('buildOverviewModel 健康度主卡', () => {
  it('有实测估算时健康度与副标题取估算/设计', () => {
    const m = buildOverviewModel(doc(), 'zh', zh);
    expect(m.pctText).toBe('92');
    expect(m.subText).toBe('4600 / 5000 mAh');
    expect(m.tempText).toBe('温度 31.5 ℃');
  });

  it('没有实测估算时回退内核健康度', () => {
    const m = buildOverviewModel(doc({ est_mah: null }), 'zh', zh);
    expect(m.pctText).toBe('96');
  });
});

describe('buildOverviewModel 趋势与终点预测', () => {
  it('显著衰减展示每周斜率与周级终点预测', () => {
    const m = buildOverviewModel(
      doc({ trend_state: 'significant', trend_mah_per_week: -3.5, weeks_to_80: 4 }),
      'zh',
      zh
    );
    expect(m.trendText).toBe('每周衰减 3.5 mAh');
    expect(m.lifeText).toBe('约 4 周后降至 80%');
  });

  it('终点预测按月换算，英文单周用单数', () => {
    const months = buildOverviewModel(
      doc({ trend_state: 'significant', trend_mah_per_week: -3.5, weeks_to_80: 100 }),
      'en',
      en
    );
    expect(months.lifeText).toBe('About 23 months to 80% at this rate');
    const week = buildOverviewModel(doc({ weeks_to_80: 1 }), 'en', en);
    expect(week.lifeText).toBe('About 1 week to 80% at this rate');
  });

  it('缺 weeks_to_80 或非正数时不显示预测', () => {
    expect(buildOverviewModel(doc(), 'zh', zh).lifeText).toBeNull();
    expect(buildOverviewModel(doc({ weeks_to_80: 0 }), 'zh', zh).lifeText).toBeNull();
  });

  it('稳定与积累中趋势的文案与天数', () => {
    expect(buildOverviewModel(doc({ trend_state: 'stable' }), 'zh', zh).trendText).toBe(
      '无显著变化'
    );
    const ins = buildOverviewModel(
      doc({ trend_state: 'insufficient', trend_span_day: 5 }),
      'zh',
      zh
    );
    expect(ins.trendText).toBe('数据积累中 · 5/21 天');
  });
});
