// 底栏拖拽的 snap 恢复控制器：拖动时会把 #app 的 scroll-snap-type 临时设为 none，
// 松手后靠一个延时计时器恢复。旧实现里 startDrag 无条件 clearTimeout 却不恢复样式：
// 若紧接着这一下只点不拖（settleDrag 的 !drag.moved 早退），#app 会永久停在 none。
// 这里把「待恢复计时器 + 恢复动作」收拢成一个小控制器，任何路径结束都能恢复：
// - restore(): 有待恢复计时器就取消并立即恢复样式（startDrag / 未移动的 settle 调用）；
// - hold(ms):  保持 none 到 ms 后再恢复（同一时间只保留一个待定计时器）；
// - release(): 只取消计时器、不改样式（切页交给 App 的弹簧动画自己恢复 snap）。
function createSnapRestore(options) {
  const app = options.app;
  const setTimer = options.setTimer;
  const clearTimer = options.clearTimer;
  let timer = 0;

  function restore() {
    if (!timer) return false;
    clearTimer(timer);
    timer = 0;
    app.style.scrollSnapType = '';
    return true;
  }

  function hold(ms) {
    if (timer) clearTimer(timer);
    timer = setTimer(function () {
      timer = 0;
      app.style.scrollSnapType = '';
    }, ms);
  }

  function release() {
    if (!timer) return;
    clearTimer(timer);
    timer = 0;
  }

  return { restore: restore, hold: hold, release: release };
}

export { createSnapRestore };
