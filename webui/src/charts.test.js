import { describe, expect, it } from 'vitest';
import { pickPairs, svgBars, svgLine, svgScatter, svgTsLine } from './charts.js';

// 会话柱（svgBars）的选中态：选中柱用系列色、其余柱换成低强调色、无高亮圈。
// 高亮圈是折线/散点单点定位用的，柱有整段填充，用颜色层次区分即可。
const rows = [{ delta_pct: 10 }, { delta_pct: 30 }, { delta_pct: 20 }];
const ACCENT = 'var(--accent)';
const DIM = 'var(--chart-dim)';

function barTags(svg) {
  return svg.match(/<rect class="bar"[^>]*>/g) || [];
}

function fills(tags) {
  return tags.map(function (tag) {
    return /style="fill:([^"]+)"/.exec(tag)[1];
  });
}

describe('svgBars 会话柱选中态', () => {
  it('无选中时全部用系列色、不画高亮圈', () => {
    const svg = svgBars(rows, 320, 110, ACCENT, null, DIM);
    const tags = barTags(svg);
    expect(fills(tags)).toEqual([ACCENT, ACCENT, ACCENT]);
    expect(svg).not.toContain('pt-sel');
    expect(svg).toContain('data-i="2"');
  });

  it('选中一柱时该柱用系列色，其余两柱换成低强调色且无高亮圈', () => {
    const svg = svgBars(rows, 320, 110, ACCENT, 1, DIM);
    const tags = barTags(svg);
    expect(tags).toHaveLength(3);
    expect(fills(tags)).toEqual([DIM, ACCENT, DIM]);
    expect(svg).not.toContain('pt-sel');
  });

  it('不传低强调色时全部保持系列色（向后兼容）', () => {
    const svg = svgBars(rows, 320, 110, ACCENT, 1);
    expect(fills(barTags(svg))).toEqual([ACCENT, ACCENT, ACCENT]);
  });

  it('只有一条会话且被选中时全部用系列色', () => {
    const svg = svgBars([{ delta_pct: 42 }], 320, 110, ACCENT, 0, DIM);
    expect(fills(barTags(svg))).toEqual([ACCENT]);
    expect(svg).not.toContain('pt-sel');
  });

  it('越界/非整数选中下标不生效', () => {
    [99, -1, 1.5, null, undefined, 'x'].forEach(function (sel) {
      const svg = svgBars(rows, 320, 110, ACCENT, sel, DIM);
      expect(fills(barTags(svg))).toEqual([ACCENT, ACCENT, ACCENT]);
      expect(svg).not.toContain('pt-sel');
    });
  });

  it('没有有效 delta_pct 时返回空串', () => {
    expect(svgBars([], 320, 110, ACCENT)).toBe('');
    expect(
      svgBars([{ delta_pct: undefined }, { foo: 1 }, { delta_pct: 'x' }], 320, 110, ACCENT)
    ).toBe('');
  });

  it('最大点数 120 时柱宽占槽位 75%、保留可见间隔，圆角对齐 4dp', () => {
    const many = Array.from({ length: 120 }, function (_, i) {
      return { delta_pct: (i % 60) + 1 };
    });
    const tags = barTags(svgBars(many, 320, 110, ACCENT, null, DIM));
    expect(tags).toHaveLength(120);
    const widths = tags.map(function (tag) {
      return Number(/width="([\d.]+)"/.exec(tag)[1]);
    });
    // 320 / 120 * 0.75 = 2.0；小于槽位 2.67，柱间仍有约 0.7px 间隔。
    expect(Math.min.apply(null, widths)).toBeCloseTo(2.0, 5);
    expect(Math.max.apply(null, widths)).toBeCloseTo(2.0, 5);
    expect(tags.every((tag) => tag.includes('rx="4"'))).toBe(true);
  });
});

// 数据点计数：命中圈的 data-i 只在真正绘制的点出现，用来断言 null 行被跳过。
function pointCount(svg) {
  return (svg.match(/data-i=/g) || []).length;
}

describe('pickPairs 缺失值过滤', () => {
  it('null / undefined / 空串 / 缺键直接跳过，不经过 Number(null)=0', () => {
    const rows = [
      { ts: 1000, mah: 4400 },
      { ts: 2000, mah: null },
      { ts: null, mah: 4500 },
      { ts: 3000, mah: '' },
      { ts: '', mah: 4600 },
      { mah: 4700 },
      { ts: 4000 },
      { ts: 'x', mah: 4500 },
      { ts: 5000, mah: 'y' },
      { ts: 6000, mah: 4800 },
    ];
    expect(pickPairs(rows, 'ts', 'mah')).toEqual([
      { x: 1000, y: 4400, row: rows[0] },
      { x: 6000, y: 4800, row: rows[9] },
    ]);
  });

  it('行本身为 null 时跳过', () => {
    expect(pickPairs([null, { ts: 1, v: 2 }, undefined], 'ts', 'v')).toEqual([
      { x: 1, y: 2, row: { ts: 1, v: 2 } },
    ]);
  });

  it('全无效数据返回空数组', () => {
    expect(pickPairs([{ ts: null, v: null }, { ts: '', v: '' }, { foo: 1 }], 'ts', 'v')).toEqual(
      []
    );
  });
});

describe('svgLine / svgTsLine / svgScatter 对缺失值降级', () => {
  it('全无效数据返回空串（由调用方决定占位/隐藏）', () => {
    expect(svgLine([{ ts: null, mah: null }], 320, 110, ACCENT)).toBe('');
    expect(svgLine([], 320, 110, ACCENT)).toBe('');
    expect(svgTsLine([{ ts: 1, v: null }], 320, 110, ACCENT, 'v')).toBe('');
    expect(svgScatter([{ ts: 1, uv: '' }], 320, 110, ACCENT)).toBe('');
    expect(svgScatter([], 320, 110, ACCENT)).toBe('');
  });

  it('混有 null 行的折线只画有效点，且不会把 ts=0 假点算进量程', () => {
    const data = [
      { ts: null, mah: 9999 },
      { ts: 1000, mah: 4400 },
      { ts: null, mah: 4500 },
      { ts: 2000, mah: null },
      { ts: 3000, mah: 4600 },
    ];
    const svg = svgLine(data, 320, 110, ACCENT);
    expect(svg).toContain('<svg');
    expect(pointCount(svg)).toBe(2);
    // 两点 x 归一化后分别落在 5% / 95% 两端：ts=0 的假点不存在。
    expect(svg).toContain('M16.0,');
    expect(svg).toContain('304.0,');
  });

  it('svgTsLine 混有 null 行时只画有效点', () => {
    const svg = svgTsLine(
      [
        { ts: 0, v: 1 },
        { ts: 1, v: null },
        { ts: 2, v: 3 },
      ],
      320,
      110,
      ACCENT,
      'v'
    );
    expect(pointCount(svg)).toBe(2);
  });

  it('svgScatter 混有 null 行时只画有效点', () => {
    const svg = svgScatter(
      [
        { ts: 1, uv: 4000 },
        { ts: 2, uv: null },
        { ts: 3, uv: 4200 },
      ],
      320,
      110,
      ACCENT
    );
    expect(pointCount(svg)).toBe(2);
  });
});
