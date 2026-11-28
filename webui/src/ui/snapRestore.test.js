import { describe, expect, it } from 'vitest';
import { createSnapRestore } from './snapRestore.js';

// 手动 timer 表：精确控制「待恢复计时器」是否还存在，不依赖 fake timers。
function harness() {
  const timers = new Map();
  let nextId = 1;
  const app = { style: { scrollSnapType: '' } };
  const snap = createSnapRestore({
    app: app,
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
  return { app: app, snap: snap, timers: timers, fire: fire };
}

describe('createSnapRestore 底栏 snap 恢复', () => {
  it('hold 到点后恢复 scroll-snap-type，且计时器被消费', () => {
    const h = harness();
    h.app.style.scrollSnapType = 'none';
    h.snap.hold(380);
    expect(h.timers.size).toBe(1);
    expect(h.fire()).toBe(380);
    expect(h.app.style.scrollSnapType).toBe('');
    expect(h.timers.size).toBe(0);
  });

  it('restore：有待恢复计时器时取消并立即恢复（startDrag 重入/未移动的 settle）', () => {
    const h = harness();
    h.app.style.scrollSnapType = 'none';
    h.snap.hold(380);
    expect(h.snap.restore()).toBe(true);
    expect(h.timers.size).toBe(0);
    expect(h.app.style.scrollSnapType).toBe('');
    // 再调一次无计时器可取消，样式保持不变。
    expect(h.snap.restore()).toBe(false);
    expect(h.app.style.scrollSnapType).toBe('');
  });

  it('没有待恢复计时器时 restore 不碰样式（切页动画自己管理 snap）', () => {
    const h = harness();
    h.app.style.scrollSnapType = 'none';
    expect(h.snap.restore()).toBe(false);
    expect(h.app.style.scrollSnapType).toBe('none');
  });

  it('重复 hold 只保留一个待定计时器，旧计时器被取消', () => {
    const h = harness();
    h.app.style.scrollSnapType = 'none';
    h.snap.hold(380);
    h.snap.hold(380);
    expect(h.timers.size).toBe(1);
    h.snap.restore();
    expect(h.timers.size).toBe(0);
  });

  it('release 只取消计时器、不改样式（交给 App 切页弹簧动画）', () => {
    const h = harness();
    h.app.style.scrollSnapType = 'none';
    h.snap.hold(380);
    h.snap.release();
    expect(h.timers.size).toBe(0);
    expect(h.app.style.scrollSnapType).toBe('none');
    // release 后再 restore 也不改样式，动画结束时自己会恢复。
    expect(h.snap.restore()).toBe(false);
  });

  it('「上一段吸附的恢复计时器还没到点就重新起手」不会永久停在 none', () => {
    const h = harness();
    h.app.style.scrollSnapType = 'none';
    h.snap.hold(380); // 上一段同页吸附的恢复计时器
    h.snap.restore(); // 新的 startDrag：先恢复
    expect(h.app.style.scrollSnapType).toBe('');
    h.snap.restore(); // 这一下没拖动就松手（settleDrag !moved）：无计时器可取消
    expect(h.app.style.scrollSnapType).toBe('');
    expect(h.timers.size).toBe(0);
  });
});
