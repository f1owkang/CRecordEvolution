import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { CB_OBSERVE_MS, hostColorSupported, run, syncModuleLang } from './core.js';

const CB_PREFIX = '__cr_cb_';

function cbNames() {
  const win = globalThis.window || {};
  return Object.keys(win).filter(function (key) {
    return key.indexOf(CB_PREFIX) === 0;
  });
}

// 假宿主：ksu 全局对象 + window（cbName 协议把回调挂在 window 上）。
function installHost(exec) {
  globalThis.window = {};
  globalThis.ksu = { exec: exec };
}

function installCss(vars) {
  globalThis.document = { documentElement: {} };
  globalThis.getComputedStyle = function () {
    return {
      getPropertyValue: function (name) {
        return vars[name] || '';
      },
    };
  };
}

beforeEach(function () {
  globalThis.window = {};
});

afterEach(function () {
  vi.useRealTimers();
  delete globalThis.window;
  delete globalThis.ksu;
  delete globalThis.document;
  delete globalThis.getComputedStyle;
});

describe('run 传输级联', () => {
  it('回调型宿主：cbName 回调成功，exec 只调一次且全局回调被清理', async () => {
    vi.useFakeTimers();
    let cbArg = null;
    const exec = vi.fn(function (cmd, options, name) {
      cbArg = name;
      globalThis.window[name](0, '{"ok":true}', '');
    });
    installHost(exec);

    const result = await run('/bin/batteryd json');

    expect(result).toEqual({ errno: 0, stdout: '{"ok":true}', stderr: '' });
    expect(exec).toHaveBeenCalledTimes(1);
    expect(exec.mock.calls[0][0]).toBe('/bin/batteryd json');
    expect(cbArg).toMatch(/^__cr_cb_/);
    expect(typeof globalThis.window[cbArg]).toBe('undefined');
    expect(cbNames()).toEqual([]);
    // 同步回包后不能留下空转的观察窗计时器（也不该留下总超时计时器）。
    expect(vi.getTimerCount()).toBe(0);
  });

  it('promise-only 宿主：采纳返回的 Promise，不等待 4s 观察窗、不重试', async () => {
    vi.useFakeTimers();
    const exec = vi.fn(function () {
      return Promise.resolve({ errno: 0, stdout: '{"a":1}', stderr: '' });
    });
    installHost(exec);

    // fake timers 下若实现傻等观察窗，这个 await 会永远挂起（测试超时即回归）。
    const result = await run('cmd');

    expect(result).toEqual({ errno: 0, stdout: '{"a":1}', stderr: '' });
    expect(exec).toHaveBeenCalledTimes(1);
    expect(cbNames()).toEqual([]);
  });

  it('cbName 同步抛异常：errno -1（不炸掉级联），且没有全局回调残留', async () => {
    vi.useFakeTimers();
    const exec = vi.fn(function () {
      throw new Error('boom');
    });
    installHost(exec);

    const pending = run('cmd');
    await vi.advanceTimersByTimeAsync(1200);
    const result = await pending;

    expect(result.errno).toBe(-1);
    expect(exec.mock.calls.length).toBeGreaterThanOrEqual(1);
    expect(cbNames()).toEqual([]);
  });

  it('cbName 不回包也不返回 thenable：观察窗后交棒，由返回 Promise 的传输成功', async () => {
    vi.useFakeTimers();
    const exec = vi.fn(function () {
      if (arguments.length >= 3) return undefined; // 只认命令行、不认 cbName
      return Promise.resolve({ errno: 0, stdout: 'ok', stderr: '' });
    });
    installHost(exec);

    const pending = run('cmd');
    await vi.advanceTimersByTimeAsync(CB_OBSERVE_MS + 400);
    const result = await pending;

    expect(result).toEqual({ errno: 0, stdout: 'ok', stderr: '' });
    expect(exec).toHaveBeenCalledTimes(2);
    expect(cbNames()).toEqual([]);
  });

  it('所有传输都不回包：最终 errno -1，且（总超时路径）全局回调被清理', async () => {
    vi.useFakeTimers();
    const exec = vi.fn(function () {
      /* 既不同步回包，也不返回 thenable */
    });
    installHost(exec);

    const pending = run('cmd');
    await vi.advanceTimersByTimeAsync(CB_OBSERVE_MS + 1000);
    await vi.advanceTimersByTimeAsync(20000);
    const result = await pending;

    expect(result.errno).toBe(-1);
    expect(exec.mock.calls.length).toBeGreaterThanOrEqual(3);
    expect(cbNames()).toEqual([]);
  });

  it('thenable 永不落地：总超时把已注册的全局回调一并清理', async () => {
    vi.useFakeTimers();
    let cbName = null;
    const exec = vi.fn(function (cmd, options, name) {
      cbName = name;
      return new Promise(function () {
        /* never resolves */
      });
    });
    installHost(exec);

    const pending = run('cmd');
    expect(cbName).toMatch(/^__cr_cb_/);
    expect(typeof globalThis.window[cbName]).toBe('function');

    await vi.advanceTimersByTimeAsync(20000);
    const result = await pending;

    expect(result.errno).toBe(-1);
    expect(result.stderr).toBe('timeout');
    expect(typeof globalThis.window[cbName]).toBe('undefined');
    expect(cbNames()).toEqual([]);
  });
});

describe('hostColorSupported 取色能力探测', () => {
  it('宿主提供可解析的 --background（hex / rgb）才认为支持取色', () => {
    installCss({ '--primary': '#6750a4', '--background': 'rgb(18 18 18)' });
    expect(hostColorSupported()).toBe(true);
    installCss({ '--primary': 'rgb(103 80 164)', '--background': '#fff' });
    expect(hostColorSupported()).toBe(true);
  });

  it('缺变量、空值或 --background 不可解析时返回 false', () => {
    installCss({ '--primary': '#6750a4', '--background': '' });
    expect(hostColorSupported()).toBe(false);
    installCss({ '--primary': '', '--background': '#101010' });
    expect(hostColorSupported()).toBe(false);
    installCss({ '--primary': '#6750a4', '--background': 'var(--surface)' });
    expect(hostColorSupported()).toBe(false);
  });

  it('读取 computed style 抛异常时安全降级', () => {
    globalThis.document = { documentElement: {} };
    globalThis.getComputedStyle = function () {
      throw new Error('no style');
    };
    expect(hostColorSupported()).toBe(false);
  });
});

describe('syncModuleLang 入参白名单', () => {
  it('非 zh/en 入参直接丢弃：不拼进 shell、不调用 exec', () => {
    const exec = vi.fn();
    installHost(exec);
    syncModuleLang('en; id');
    syncModuleLang('zh && reboot');
    syncModuleLang('zh ');
    syncModuleLang('');
    expect(exec).not.toHaveBeenCalled();
  });
});
