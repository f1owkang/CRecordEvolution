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
    expect(row.sub).toContain('80→58%');
  });

  it('未知事件返回 null，不生成空行', () => {
    expect(toTimelineRow({ kind: 'rest' }, t, 'zh')).toBeNull();
  });
});
