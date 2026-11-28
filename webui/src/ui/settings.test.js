import { describe, expect, it } from 'vitest';
import { diagOutcome } from './settings.jsx';

// 诊断按钮的提示只存 i18n key：state 里若存已翻译字符串，切语言后提示会停在旧语言。
describe('diagOutcome 诊断复制结果', () => {
  it('命令失败或空输出 → diag_error，带标记 error', () => {
    expect(diagOutcome({ errno: 1, stdout: '', stderr: 'x' }, false)).toEqual({
      busy: false,
      message: { key: 'diag_error' },
      text: '',
      error: true,
    });
    expect(diagOutcome({ errno: 0, stdout: '   ' }, false).message).toEqual({ key: 'diag_error' });
    expect(diagOutcome(null, false).message).toEqual({ key: 'diag_error' });
  });

  it('复制成功 → diag_copied，无兜底文本', () => {
    expect(diagOutcome({ errno: 0, stdout: 'diag text' }, true)).toEqual({
      busy: false,
      message: { key: 'diag_copied' },
      text: '',
      error: false,
    });
  });

  it('复制失败 → diag_copy_fail，并把文本放进只读文本框兜底', () => {
    const state = diagOutcome({ errno: 0, stdout: 'diag text' }, false);
    expect(state.message).toEqual({ key: 'diag_copy_fail' });
    expect(state.text).toBe('diag text');
    expect(state.error).toBe(true);
  });

  it('message 始终是 {key} 而不是已翻译字符串（切语言后渲染时再翻译）', () => {
    const states = [
      diagOutcome({ errno: 1 }, false),
      diagOutcome({ errno: 0, stdout: 'ok' }, true),
      diagOutcome({ errno: 0, stdout: 'ok' }, false),
    ];
    states.forEach(function (state) {
      expect(typeof state.message).toBe('object');
      expect(typeof state.message.key).toBe('string');
      expect(state.message.key.length).toBeGreaterThan(0);
    });
  });
});
