import { describe, expect, it } from 'vitest';
import { CMD_HISTORY, HIST_PAGE_LIMIT } from '../core.js';
import {
  buildHistoryCmd,
  createHistoryRequestTracker,
  historyBefore,
  historyEventKey,
  mapHistoryError,
  mergeHistoryEvents,
  parseHistoryResult,
} from './historyCommand.js';

const DEFAULT_CMD = CMD_HISTORY + ' --days 30 --limit ' + HIST_PAGE_LIMIT + ' --type all';

function okDoc(overrides) {
  return JSON.stringify(Object.assign({ events: [], summary: {}, trends: {} }, overrides));
}

describe('buildHistoryCmd 参数白名单', () => {
  it('默认值：30 天 / 全部类型 / 不带游标', () => {
    expect(buildHistoryCmd({})).toBe(DEFAULT_CMD);
    expect(buildHistoryCmd()).toBe(DEFAULT_CMD);
  });

  it('合法参数按固定模板拼接', () => {
    expect(buildHistoryCmd({ days: 7, type: 'charge', before: 1700000000 })).toBe(
      CMD_HISTORY + ' --days 7 --limit ' + HIST_PAGE_LIMIT + ' --type charge --before 1700000000'
    );
    expect(buildHistoryCmd({ days: 90, type: 'discharge' })).toBe(
      CMD_HISTORY + ' --days 90 --limit ' + HIST_PAGE_LIMIT + ' --type discharge'
    );
  });

  it('天数/类型白名单外回退默认，不会把任意字符串拼进命令', () => {
    expect(buildHistoryCmd({ days: 0, type: 'everything' })).toBe(DEFAULT_CMD);
    expect(buildHistoryCmd({ days: 7.5, type: 'ALL' })).toBe(DEFAULT_CMD);
    expect(buildHistoryCmd({ days: '7; rm -rf /', type: 'all; id' })).toBe(DEFAULT_CMD);
    expect(buildHistoryCmd({ days: null, type: null })).toBe(DEFAULT_CMD);
  });

  it('游标只接受安全正整数，非法值按无游标处理', () => {
    const unsafe = [0, -1, 1.5, '123', NaN, Infinity, Number.MAX_SAFE_INTEGER + 2, null, undefined];
    unsafe.forEach(function (before) {
      const cmd = buildHistoryCmd({ days: 7, type: 'all', before: before });
      expect(cmd).not.toContain('--before');
      expect(cmd).toBe(CMD_HISTORY + ' --days 7 --limit ' + HIST_PAGE_LIMIT + ' --type all');
    });
    const injected = buildHistoryCmd({ before: '1; id' });
    expect(injected).not.toContain('--before');
    expect(injected).not.toContain('id');
  });

  it('historyBefore 只从已加载数据的 next_before 读安全正整数', () => {
    expect(historyBefore({ next_before: 1700000000 })).toBe(1700000000);
    expect(historyBefore({ next_before: '1700000000' })).toBe(0);
    expect(historyBefore({ next_before: -1 })).toBe(0);
    expect(historyBefore({ next_before: 1.5 })).toBe(0);
    expect(historyBefore(null)).toBe(0);
  });
});

describe('mapHistoryError / parseHistoryResult 错误模型', () => {
  it('成功结果返回 doc、错误为 null', () => {
    const result = { errno: 0, stdout: okDoc({ events: [{ kind: 'charge', ts: 1 }] }), stderr: '' };
    expect(mapHistoryError(result)).toBeNull();
    expect(parseHistoryResult(result).doc.events).toHaveLength(1);
  });

  it('errno 非 0 映射成 err_run（带 stderr 细节，不显示裸 errno）', () => {
    expect(mapHistoryError({ errno: 1, stdout: '', stderr: 'boom' })).toEqual({
      key: 'err_run',
      vars: { detail: 'boom' },
    });
    expect(mapHistoryError({ errno: 2, stdout: '', stderr: '' })).toEqual({
      key: 'err_run',
      vars: { detail: 'errno 2' },
    });
    expect(mapHistoryError(null)).toEqual({ key: 'err_run', vars: { detail: 'errno -1' } });
  });

  it('空输出映射 err_noout', () => {
    expect(mapHistoryError({ errno: 0, stdout: '   ', stderr: '' })).toEqual({ key: 'err_noout' });
  });

  it('非法 JSON 映射 err_json', () => {
    expect(mapHistoryError({ errno: 0, stdout: '{bad', stderr: '' })).toEqual({ key: 'err_json' });
  });

  it('结构不对（非对象 / events 不是数组）映射 err_struct', () => {
    expect(mapHistoryError({ errno: 0, stdout: 'null', stderr: '' })).toEqual({
      key: 'err_struct',
    });
    expect(mapHistoryError({ errno: 0, stdout: '123', stderr: '' })).toEqual({ key: 'err_struct' });
    expect(mapHistoryError({ errno: 0, stdout: '{"events":"x"}', stderr: '' })).toEqual({
      key: 'err_struct',
    });
  });
});

describe('createHistoryRequestTracker 请求代际', () => {
  it('reset 递增代际并作废在途追加分页', () => {
    const tracker = createHistoryRequestTracker();
    tracker.setKey('7|all|records');
    const append = tracker.begin(false);
    expect(tracker.isAppendInFlight()).toBe(true);
    // 用户切筛选：渲染先同步新参数键，effect 里再发整页重载。
    tracker.setKey('30|charge|records');
    const reset = tracker.begin(true);
    // 旧的追加分页（旧参数键）落地被丢弃，且不会清掉新的在途状态。
    expect(tracker.accept(append)).toBe(false);
    expect(tracker.settle(append)).toBe(false);
    expect(tracker.isPendingReset()).toBe(true);
    expect(tracker.settle(reset)).toBe(true);
    expect(tracker.isPendingReset()).toBe(false);
  });

  it('参数键变化后迟到的响应不落地', () => {
    const tracker = createHistoryRequestTracker();
    tracker.setKey('7|all|records');
    const token = tracker.begin(true);
    tracker.setKey('90|all|records');
    expect(tracker.accept(token)).toBe(false);
    expect(tracker.settle(token)).toBe(false);
    expect(tracker.isPendingReset()).toBe(true);
  });

  it('同一参数键下追加成功会清掉在途标记，重复点击不会并行两条', () => {
    const tracker = createHistoryRequestTracker();
    tracker.setKey('30|all|records');
    const token = tracker.begin(false);
    expect(tracker.isAppendInFlight()).toBe(true);
    expect(tracker.settle(token)).toBe(true);
    expect(tracker.isAppendInFlight()).toBe(false);
  });
});

describe('historyEventKey / mergeHistoryEvents 事件身份', () => {
  it('同 kind 同 ts 但起始信息不同的两条会话不能互相吞掉', () => {
    const a = { kind: 'charge', ts: 100, charge: { start_ts: 10, end_ts: 100, start_cap: 20 } };
    const b = { kind: 'charge', ts: 100, charge: { start_ts: 40, end_ts: 100, start_cap: 75 } };
    expect(historyEventKey(a)).not.toBe(historyEventKey(b));
    const merged = mergeHistoryEvents(
      { events: [a], has_more: true, next_before: 100, summary: {}, trends: {} },
      { events: [b], has_more: false, next_before: 0, summary: {}, trends: {} }
    );
    expect(merged.events).toHaveLength(2);
  });

  it('同 kind 同 ts 的放电行按 secs/start_cap 区分', () => {
    const a = { kind: 'discharge', ts: 200, discharge: { ts: 200, secs: 600, start_cap: 80 } };
    const b = { kind: 'discharge', ts: 200, discharge: { ts: 200, secs: 300, start_cap: 80 } };
    expect(historyEventKey(a)).not.toBe(historyEventKey(b));
  });

  it('完全相同的重叠事件只保留首次出现的一份', () => {
    const event = { kind: 'charge', ts: 100, charge: { start_ts: 10, end_ts: 100 } };
    const merged = mergeHistoryEvents(
      { events: [event], has_more: true, next_before: 100, summary: {}, trends: {} },
      {
        events: [event, event],
        has_more: false,
        next_before: 0,
        summary: { n: 1 },
        trends: { t: 2 },
      }
    );
    expect(merged.events).toEqual([event]);
    expect(merged.summary).toEqual({ n: 1 });
    expect(merged.trends).toEqual({ t: 2 });
    expect(merged.has_more).toBe(false);
  });

  it('缺少子对象/字段时退回 kind:ts（向后兼容旧 payload）', () => {
    expect(historyEventKey({ kind: 'charge', ts: 5 })).toBe('charge:5');
    expect(historyEventKey({ kind: 'discharge', ts: 6 })).toBe('discharge:6');
    expect(historyEventKey({ kind: 'charge', ts: 5, charge: {} })).toBe('charge:5:c');
  });
});
