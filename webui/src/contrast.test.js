import { describe, expect, it } from 'vitest';
import {
  contrastRatio,
  dimToContrast,
  ensureContrast,
  parseCssColor,
  relativeLuminance,
} from './contrast.js';

describe('parseCssColor', () => {
  it('解析 3/4/6/8 位十六进制', () => {
    expect(parseCssColor('#fff')).toEqual({ r: 255, g: 255, b: 255 });
    expect(parseCssColor('#0B0B0D')).toEqual({ r: 11, g: 11, b: 13 });
    expect(parseCssColor('#11223380')).toEqual({ r: 17, g: 34, b: 51 });
  });

  it('解析 rgb/rgba 的逗号与空格写法', () => {
    expect(parseCssColor('rgb(255, 0, 1)')).toEqual({ r: 255, g: 0, b: 1 });
    expect(parseCssColor('rgba(1 2 3 / 0.5)')).toEqual({ r: 1, g: 2, b: 3 });
  });

  it('空值/不支持的写法返回 null', () => {
    expect(parseCssColor('')).toBeNull();
    expect(parseCssColor('var(--primary)')).toBeNull();
    expect(parseCssColor('#12345')).toBeNull();
  });
});

describe('contrastRatio', () => {
  it('黑白对比度为 21，同色为 1', () => {
    expect(contrastRatio('#000', '#fff')).toBeCloseTo(21, 5);
    expect(contrastRatio('#fff', '#fff')).toBeCloseTo(1, 5);
  });

  it('非法颜色返回 null', () => {
    expect(contrastRatio('var(--x)', '#fff')).toBeNull();
  });

  it('相对亮度按 WCAG 公式计算', () => {
    expect(relativeLuminance({ r: 0, g: 0, b: 0 })).toBeCloseTo(0, 6);
    expect(relativeLuminance({ r: 255, g: 255, b: 255 })).toBeCloseTo(1, 6);
  });
});

describe('ensureContrast 取色主题纠偏', () => {
  it('已达标时返回 null（保持宿主原值）', () => {
    expect(ensureContrast('#ffffff', '#0b0b0d', '#e6e6e6', 4.5)).toBeNull();
    expect(ensureContrast('#00ff00', '#000000', '#ffffff', 4.5)).toBeNull();
  });

  it('Monochrome 下 tertiary 纯黑抬到可读灰度', () => {
    const fixed = ensureContrast('#000000', '#0b0b0d', '#e6e6e6', 4.5);
    expect(fixed).toMatch(/^#[0-9a-f]{6}$/);
    expect(contrastRatio(fixed, '#0b0b0d')).toBeGreaterThanOrEqual(4.5);
  });

  it('Monochrome 下 secondary 深灰同样抬亮', () => {
    const fixed = ensureContrast('#474747', '#0b0b0d', '#e6e6e6', 4.5);
    expect(fixed).not.toBe('#474747');
    expect(contrastRatio(fixed, '#0b0b0d')).toBeGreaterThanOrEqual(4.5);
  });

  it('浅色卡片上的过浅颜色会被压暗（低对比度方向对称）', () => {
    const fixed = ensureContrast('#fefefe', '#f4fbfb', '#161d1d', 4.5);
    expect(fixed).toMatch(/^#[0-9a-f]{6}$/);
    expect(contrastRatio(fixed, '#f4fbfb')).toBeGreaterThanOrEqual(4.5);
  });

  it('toward 也达不到阈值时用黑/白兜底', () => {
    const fixed = ensureContrast('#000000', '#0b0b0d', '#0b0b0d', 4.5);
    expect(fixed).toMatch(/^#[0-9a-f]{6}$/);
    expect(contrastRatio(fixed, '#0b0b0d')).toBeGreaterThanOrEqual(4.5);
  });

  it('颜色或背景无法解析时返回 null', () => {
    expect(ensureContrast('var(--tertiary)', '#0b0b0d', '#e6e6e6', 4.5)).toBeNull();
    expect(ensureContrast('#000000', 'var(--card)', '#e6e6e6', 4.5)).toBeNull();
  });
});

describe('dimToContrast 低强调色', () => {
  it('深色底上把白色压到 3:1 的最深一档', () => {
    const dim = dimToContrast('#ffffff', '#0b0b0d', 3);
    expect(dim).toMatch(/^#[0-9a-f]{6}$/);
    const ratio = contrastRatio(dim, '#0b0b0d');
    expect(ratio).toBeGreaterThanOrEqual(3);
    expect(ratio).toBeLessThan(3.4);
    expect(dim).not.toBe('#ffffff');
  });

  it('浅色底上把深绿朝白色提亮到 3:1，方向与深色底相反', () => {
    const dim = dimToContrast('#1a7f37', '#ffffff', 3);
    expect(dim).toMatch(/^#[0-9a-f]{6}$/);
    const ratio = contrastRatio(dim, '#ffffff');
    expect(ratio).toBeGreaterThanOrEqual(3);
    expect(ratio).toBeLessThan(3.4);
    expect(relativeLuminance(parseCssColor(dim))).toBeGreaterThan(
      relativeLuminance(parseCssColor('#1a7f37'))
    );
  });

  it('彩色强调色同样压到刚过阈值', () => {
    const dim = dimToContrast('#4ade80', '#151a23', 3);
    const ratio = contrastRatio(dim, '#151a23');
    expect(ratio).toBeGreaterThanOrEqual(3);
    expect(ratio).toBeLessThan(3.4);
  });

  it('浅色的 1.8:1 container 档比 3:1 更浅', () => {
    const light = dimToContrast('#1a7f37', '#ffffff', 1.8);
    const deep = dimToContrast('#1a7f37', '#ffffff', 3);
    expect(relativeLuminance(parseCssColor(light))).toBeGreaterThan(
      relativeLuminance(parseCssColor(deep))
    );
    const ratio = contrastRatio(light, '#ffffff');
    expect(ratio).toBeGreaterThanOrEqual(1.8);
    expect(ratio).toBeLessThan(2.1);
  });

  it('前景本身低于阈值或颜色无效时返回 null', () => {
    expect(dimToContrast('#333333', '#0b0b0d', 3)).toBeNull();
    expect(dimToContrast('var(--accent)', '#0b0b0d', 3)).toBeNull();
    expect(dimToContrast('#ffffff', 'var(--card)', 3)).toBeNull();
  });
});
