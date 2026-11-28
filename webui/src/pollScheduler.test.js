import { describe, expect, it } from 'vitest';
import { createPollScheduler } from './pollScheduler.js';

// 手动 timer 表：不依赖 fake timers，可以精确断言「任何时刻最多一个待定 timer」。
function harness(opts) {
  const timers = new Map();
  let nextId = 1;
  const state = { hidden: false };
  const scheduler = createPollScheduler({
    poll: opts.poll,
    nextDelay: opts.nextDelay,
    isHidden: function () {
      return state.hidden;
    },
    setTimer: function (fn, ms) {
      const id = nextId++;
      timers.set(id, { fn: fn, ms: ms });
      return id;
    },
    clearTimer: function (id) {
      timers.delete(id);
    },
  });
  function fire() {
    const first = timers.entries().next();
    expect(first.done).toBe(false);
    const id = first.value[0];
    const entry = first.value[1];
    timers.delete(id);
    entry.fn();
    return entry.ms;
  }
  return {
    scheduler: scheduler,
    timers: timers,
    state: state,
    fire: fire,
    hidden: function (v) {
      state.hidden = v;
    },
  };
}

// 让 poll 的 Promise 与微任务链跑完。
function flush() {
  return Promise.resolve().then(function () {
    return Promise.resolve();
  });
}

describe('createPollScheduler 轮询调度', () => {
  it('正常链按 nextDelay 推进：reset 立即取数，成功后按充电/非充电步长排下一次', async () => {
    let charge = false;
    const polls = [];
    const h = harness({
      poll: function () {
        polls.push(charge);
        return Promise.resolve(true);
      },
      nextDelay: function (ok) {
        if (!ok) return 999;
        return charge ? 15000 : 60000;
      },
    });
    h.scheduler.reset();
    await flush();
    expect(polls).toHaveLength(0); // reset 只排 timer，到点才取数
    expect(h.timers.size).toBe(1);
    expect(h.fire()).toBe(0);
    await flush();
    expect(polls).toHaveLength(1);
    expect(h.timers.size).toBe(1);
    charge = true;
    // fire 的是 poll#1 排的 60000；它到点后 poll#2 看到 charge=true，下一次步长变 15000。
    expect(h.fire()).toBe(60000);
    await flush();
    expect(polls).toHaveLength(2);
    expect(h.fire()).toBe(15000);
    await flush();
    expect(polls).toHaveLength(3);
    expect(h.fire()).toBe(15000);
    await flush();
    expect(polls).toHaveLength(4);
    expect(h.fire()).toBe(15000);
  });

  it('失败时按 nextDelay(false, prev) 退避，成功后退回正常步长', async () => {
    let result = false;
    const waits = [];
    const h = harness({
      poll: function () {
        return Promise.resolve(result);
      },
      nextDelay: function (good, prev) {
        const wait = good ? 60000 : Math.min((prev || 60000) * 2, 300000);
        waits.push(wait);
        return wait;
      },
    });
    h.scheduler.reset();
    expect(h.fire()).toBe(0);
    await flush();
    expect(waits).toEqual([120000]);
    expect(h.fire()).toBe(120000);
    await flush();
    expect(waits).toEqual([120000, 240000]);
    result = true;
    expect(h.fire()).toBe(240000);
    await flush();
    expect(waits).toEqual([120000, 240000, 60000]);
  });

  it('reset 时有在途 fetch：旧链 resolve 后不再调度，始终只有一个 timer', async () => {
    const pending = [];
    const h = harness({
      poll: function () {
        return new Promise(function (resolve) {
          pending.push(resolve);
        });
      },
      nextDelay: function () {
        return 60000;
      },
    });
    h.scheduler.reset();
    expect(h.fire()).toBe(0); // 第一次取数进入在途
    await flush();
    expect(pending).toHaveLength(1);
    // 在途期间 reset（隐藏回前台/手动刷新）：只能留下一条新链的 timer。
    h.scheduler.reset();
    expect(h.timers.size).toBe(1);
    // 旧链 resolve：代际作废，不得再排 timer / 再取数。
    pending[0](true);
    await flush();
    expect(pending).toHaveLength(1);
    expect(h.timers.size).toBe(1);
    // 新链的 timer 到点才开始取数。
    expect(h.fire()).toBe(0);
    await flush();
    expect(pending).toHaveLength(2);
  });

  it('hidden 时 reset 不排 timer、不取数；恢复后 reset 才继续', async () => {
    let polls = 0;
    const h = harness({
      poll: function () {
        polls += 1;
        return Promise.resolve(true);
      },
      nextDelay: function () {
        return 60000;
      },
    });
    h.hidden(true);
    h.scheduler.reset();
    expect(h.timers.size).toBe(0);
    await flush();
    expect(polls).toBe(0);
    h.hidden(false);
    h.scheduler.reset();
    expect(h.timers.size).toBe(1);
    expect(h.fire()).toBe(0);
    await flush();
    expect(polls).toBe(1);
  });

  it('取数途中变 hidden：响应落地后不再排下一次（等 visibilitychange→reset）', async () => {
    let resolvePoll;
    const h = harness({
      poll: function () {
        return new Promise(function (resolve) {
          resolvePoll = resolve;
        });
      },
      nextDelay: function () {
        return 60000;
      },
    });
    h.scheduler.reset();
    expect(h.fire()).toBe(0);
    await flush();
    h.hidden(true);
    resolvePoll(true);
    await flush();
    expect(h.timers.size).toBe(0);
  });

  it('stop 后无 timer，旧链 resolve 无副作用', async () => {
    let resolvePoll;
    const h = harness({
      poll: function () {
        return new Promise(function (resolve) {
          resolvePoll = resolve;
        });
      },
      nextDelay: function () {
        return 60000;
      },
    });
    h.scheduler.reset();
    expect(h.fire()).toBe(0);
    await flush();
    h.scheduler.stop();
    expect(h.timers.size).toBe(0);
    resolvePoll(true);
    await flush();
    expect(h.timers.size).toBe(0);
    // stop 后 reset 也不会复活。
    h.scheduler.reset();
    expect(h.timers.size).toBe(0);
  });

  it('poll 同步抛异常也按失败退避，不炸掉链', async () => {
    const waits = [];
    const h = harness({
      poll: function () {
        throw new Error('boom');
      },
      nextDelay: function (ok, prev) {
        const wait = ok ? 60000 : Math.min((prev || 60000) * 2, 300000);
        waits.push(wait);
        return wait;
      },
    });
    h.scheduler.reset();
    expect(h.fire()).toBe(0);
    await flush();
    expect(waits).toEqual([120000]);
    expect(h.timers.size).toBe(1);
  });
});
