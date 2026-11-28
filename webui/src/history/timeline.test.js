import { describe, expect, it } from 'vitest';
import { translate } from '../i18n.js';
import { toTimelineRow } from './timeline.jsx';

const t = (key, vars) => translate('zh', key, vars);

describe('toTimelineRow 记录行', () => {
  it('有效充电会话：参与估算并给出容量/温度/时长副指标', () => {
    const row = toTimelineRow(
      {
        kind: 'charge',
        charge: {
          start_ts: 1000,
          end_ts: 4600,
          start_cap: 20,
          end_cap: 80,
          delta_pct: 60,
          est_mah: 4600,
          temp_avg: 31,
          valid: true,
        },
      },
      t,
      'zh'
    );
    expect(row.title).toBe('充电会话');
    expect(row.primary).toBe('电量 20% → 80%');
    expect(row.status).toBe('参与估算');
    expect(row.statusOk).toBe(true);
    expect(row.invalid).toBe(false);
    expect(row.sub).toEqual(['估算 4600 mAh', '温度 31 ℃', '历时 60 分钟']);
    expect(row.soc).toEqual({ from: 20, to: 80, lo: 20, hi: 80 });
    expect(row.socText).toBe('电量 20% → 80%');
    expect(row.detail.length).toBeGreaterThan(0);
  });

  it('无效会话带机器可读原因时映射为原因文案', () => {
    const row = toTimelineRow(
      {
        kind: 'charge',
        charge: {
          start_ts: 0,
          end_ts: 60,
          start_cap: 50,
          end_cap: 55,
          delta_pct: 5,
          valid: false,
          invalid_reason: 'delta_lt_20',
        },
      },
      t,
      'zh'
    );
    expect(row.invalid).toBe(true);
    expect(row.statusOk).toBe(false);
    expect(row.status).toBe('电量提升不足 20%');
  });

  it('放电记录：主指标为放出电量，副指标含 SOC 变化', () => {
    const row = toTimelineRow(
      {
        kind: 'discharge',
        discharge: {
          ts: 1000,
          secs: 5400,
          uah_mah: 2100,
          start_cap: 80,
          end_cap: 58,
        },
      },
      t,
      'zh'
    );
    expect(row.title).toBe('放电记录');
    expect(row.primary).toBe('2100 mAh');
    expect(row.status).toBe('已记录');
    expect(row.sub).toEqual(['80→58%', '历时 90 分钟']);
    // 区间条与充电共用同一套尺寸，起止值仍由文字/aria 承担
    expect(row.soc).toEqual({ from: 80, to: 58, lo: 58, hi: 80 });
    expect(row.socText).toBe('电量 80% → 58%');
  });

  it('没有起止电量时不生成区间条，退回纯文字', () => {
    const row = toTimelineRow(
      { kind: 'charge', charge: { start_ts: 0, end_ts: 60, delta_pct: 25, valid: true } },
      t,
      'zh'
    );
    expect(row.soc).toBeNull();
    expect(row.socText).toBeUndefined();
    expect(row.primary).toBe('电量 +25%');
  });

  it('越界电量夹到 0~100，区间条不会画出轨道', () => {
    const row = toTimelineRow(
      {
        kind: 'discharge',
        discharge: { ts: 0, secs: 3600, uah_mah: 100, start_cap: 130, end_cap: -5 },
      },
      t,
      'zh'
    );
    expect(row.soc).toEqual({ from: 100, to: 0, lo: 0, hi: 100 });
    expect(row.socText).toBe('电量 130% → -5%');
  });

  it('未知事件返回 null，不生成空行', () => {
    expect(toTimelineRow({ kind: 'rest' }, t, 'zh')).toBeNull();
  });
});

describe('toTimelineRow 放电时长缺失/非法', () => {
  function dischargeRow(secs) {
    return toTimelineRow(
      {
        kind: 'discharge',
        discharge: { ts: 1000, secs: secs, uah_mah: 2100, start_cap: 80, end_cap: 58 },
      },
      t,
      'zh'
    );
  }

  function hasDuration(row) {
    const labels = row.detail.map(function (pair) {
      return pair[0];
    });
    return labels.indexOf('时长') >= 0;
  }

  it('secs 缺失时不渲染 NaN min，副指标与详情一致省略时长', () => {
    [undefined, null, 0, -5, 'x', NaN, Infinity].forEach(function (secs) {
      const row = dischargeRow(secs);
      expect(row).not.toBeNull();
      expect(row.sub).toEqual(['80→58%']);
      expect(hasDuration(row)).toBe(false);
      expect(JSON.stringify(row)).not.toContain('NaN');
      expect(JSON.stringify(row)).not.toContain('min');
    });
  });

  it('有效 secs 时副指标与详情共用同一判定（分钟走 i18n）', () => {
    const row = dischargeRow(5400);
    expect(row.sub).toEqual(['80→58%', '历时 90 分钟']);
    const detailMap = {};
    row.detail.forEach(function (pair) {
      detailMap[pair[0]] = pair[1];
    });
    expect(detailMap['时长']).toBe('历时 90 分钟');
    // 中文字典里时长文案是「分钟」，不再有硬编码 ' min'。
    expect(JSON.stringify(row)).not.toContain(' min');
  });

  it('不足一分钟的秒数按 0 分钟处理（不显示 0 分钟）', () => {
    const row = dischargeRow(20);
    expect(row.sub).toEqual(['80→58%']);
    expect(hasDuration(row)).toBe(false);
  });
});
