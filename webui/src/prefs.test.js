import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { readPref, writePref } from './prefs.js';

// localStorage 在 node 环境不存在：用内存 Map 模拟宿主 WebView 的 storage。
const store = new Map();

beforeEach(function () {
  store.clear();
  vi.stubGlobal('localStorage', {
    getItem: function (key) {
      return store.has(key) ? store.get(key) : null;
    },
    setItem: function (key, value) {
      store.set(key, String(value));
    },
  });
});

afterEach(function () {
  vi.unstubAllGlobals();
});

describe('readPref 偏好读取', () => {
  it('白名单内的值原样返回，白名单外的值回退默认', () => {
    store.set('cre_theme', 'dark');
    expect(readPref('cre_theme', ['auto', 'light', 'dark', 'my'], 'auto')).toBe('dark');
    store.set('cre_theme', 'purple');
    expect(readPref('cre_theme', ['auto', 'light', 'dark', 'my'], 'auto')).toBe('auto');
  });

  it('没存过值时返回默认值', () => {
    expect(readPref('cre_barmode', ['floating', 'classic'], 'floating')).toBe('floating');
  });

  it('allowed 不是数组时不做白名单，只做空值回退', () => {
    store.set('cre_lang', 'en');
    expect(readPref('cre_lang', null, 'zh')).toBe('en');
    expect(readPref('cre_missing', undefined, 'zh')).toBe('zh');
  });

  it('storage 抛异常（隐私模式/禁用）时返回默认值而不抛出', () => {
    vi.stubGlobal('localStorage', {
      getItem: function () {
        throw new Error('denied');
      },
    });
    expect(readPref('cre_theme', ['light'], 'auto')).toBe('auto');
  });
});

describe('writePref 偏好写入', () => {
  it('写入成功返回 true', () => {
    expect(writePref('cre_lang', 'zh')).toBe(true);
    expect(store.get('cre_lang')).toBe('zh');
  });

  it('storage 抛异常时返回 false 而不抛出', () => {
    vi.stubGlobal('localStorage', {
      setItem: function () {
        throw new Error('quota');
      },
    });
    expect(writePref('cre_lang', 'en')).toBe(false);
  });
});
