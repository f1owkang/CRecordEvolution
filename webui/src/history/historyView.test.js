import { describe, expect, it } from 'vitest';
import { historyErrorView, historyListBusy } from './historyView.js';

const DATA = { events: [{ kind: 'charge', ts: 1 }], has_more: false };

describe('historyErrorView 错误行展示语义', () => {
  it('没有错误时三种展示位都不出现', () => {
    expect(historyErrorView(null, null)).toEqual({
      emptyError: false,
      topError: false,
      pagingError: null,
      retryReset: false,
    });
    expect(historyErrorView(null, DATA).topError).toBe(false);
  });

  it('首屏失败（无数据）走空态错误样式', () => {
    const view = historyErrorView({ key: 'err_run', reset: true }, null);
    expect(view.emptyError).toBe(true);
    expect(view.topError).toBe(false);
    expect(view.retryReset).toBe(true);
  });

  it('已有数据 + 整页重载失败：记录与趋势视图都渲染顶部错误行', () => {
    const err = { key: 'err_run', vars: { detail: 'boom' }, reset: true };
    const records = historyErrorView(err, DATA);
    expect(records.emptyError).toBe(false);
    expect(records.topError).toBe(true);
    expect(records.pagingError).toBeNull();
    expect(records.retryReset).toBe(true);
    // 趋势视图同样不再静默（此前是最主要的静默分支）。
    expect(historyErrorView(err, DATA).topError).toBe(true);
  });

  it('「加载更早」失败：顶部错误行与列表底部就近错误行并存', () => {
    const err = { key: 'err_run', vars: { detail: 'boom' }, reset: false };
    const view = historyErrorView(err, DATA);
    expect(view.emptyError).toBe(false);
    expect(view.topError).toBe(true);
    expect(view.pagingError).toBe(err);
    expect(view.retryReset).toBe(false);
  });
});

describe('historyListBusy 旧列表禁用条件', () => {
  it('重载在途或上一轮重载失败时禁用列表交互', () => {
    expect(historyListBusy(true, null)).toBe(true);
    expect(historyListBusy(false, { key: 'err_run', reset: true })).toBe(true);
    expect(historyListBusy(false, { key: 'err_run', reset: false })).toBe(false);
    expect(historyListBusy(false, null)).toBe(false);
  });
});
