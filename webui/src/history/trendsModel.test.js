import { describe, expect, it } from 'vitest';
import { translate } from '../i18n.js';
import { buildTrendModel } from './trendsModel.js';

const t = (key, vars) => translate('zh', key, vars);

describe('buildTrendModel 长期容量档案', () => {
  it('档案点按时间升序、最新值折算为 mAh 并标注永久保留', () => {
    const model = buildTrendModel(
      {
        archive: [
          { ts: 2000, mah: 4500 },
          { ts: 1000, mah: 4600 },
        ],
      },
      t,
      'zh',
      {}
    );
    expect(
      model.archiveRows.map(function (row) {
        return row.ts;
      })
    ).toEqual([1000, 2000]);
    expect(model.archiveVal).toBe('4500 mAh');
    expect(model.archiveSvg).toContain('<svg');
    expect(model.archiveCap).toContain('2 个点');
    expect(model.archiveCap).toContain('每日汇总，永久保留');
    // 档案是独立数据源：只有档案时趋势页也要有内容。
    expect(model.hasAnyData).toBeTruthy();
  });

  it('单个档案点只给点数和说明，不拼日期区间', () => {
    const model = buildTrendModel({ archive: [{ ts: 1000, mah: 4600 }] }, t, 'zh', {});
    expect(model.archiveCap).toBe('1 个点 · 每日汇总，永久保留');
    expect(model.archiveVal).toBe('4600 mAh');
  });

  it('无档案时不出图，空文档返回 hasAnyData=false', () => {
    const empty = buildTrendModel({ archive: [] }, t, 'zh', {});
    expect(empty.archiveSvg).toBe('');
    expect(empty.archiveRows).toEqual([]);
    expect(empty.hasAnyData).toBeFalsy();
  });

  it('档案点按读数复用容量格式（时间 · mAh）', () => {
    const model = buildTrendModel({ archive: [{ ts: 1000, mah: 4600 }] }, t, 'zh', {});
    const text = model.archiveFormat(model.archiveRows[0]);
    expect(text).toContain('4600 mAh');
    expect(text).toContain('·');
  });
});
