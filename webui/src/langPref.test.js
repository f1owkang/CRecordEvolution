import { describe, expect, it } from 'vitest';
import { adoptModuleLang, parseModuleLang } from './langPref.js';

describe('parseModuleLang data/lang 解析', () => {
  it('只接受 zh/en 前缀，大小写与尾部空白容错', () => {
    expect(parseModuleLang({ errno: 0, stdout: 'zh\n' })).toBe('zh');
    expect(parseModuleLang({ errno: 0, stdout: ' EN ' })).toBe('en');
    expect(parseModuleLang({ errno: 0, stdout: 'english' })).toBe('en');
    expect(parseModuleLang({ errno: 0, stdout: 'zh-CN' })).toBe('zh');
  });

  it('无法识别的值/命令失败返回空串', () => {
    expect(parseModuleLang({ errno: 0, stdout: 'fr' })).toBe('');
    expect(parseModuleLang({ errno: 0, stdout: '' })).toBe('');
    expect(parseModuleLang({ errno: 1, stdout: 'en' })).toBe('');
    expect(parseModuleLang(null)).toBe('');
  });
});

describe('adoptModuleLang 模块语言与用户切换的竞态', () => {
  it('用户还没手动切过语言时，采纳挂载时读到的 data/lang', () => {
    expect(adoptModuleLang(false, 'zh')).toBe(true);
    expect(adoptModuleLang(false, 'en')).toBe(true);
  });

  it('用户已经手动切过语言（langTouched）时，迟到的 cat 响应必须忽略', () => {
    expect(adoptModuleLang(true, 'zh')).toBe(false);
    expect(adoptModuleLang(true, 'en')).toBe(false);
  });

  it('解析失败（空串）不覆盖当前语言', () => {
    expect(adoptModuleLang(false, '')).toBe(false);
  });
});
