// 轮询调度器：把「取数 → 按结果算下一次等待 → 排 timer」的链抽成可测试的状态机。
// 关键不变量：
// - 任何时刻最多一个待定 timer；reset/stop 会作废在途链（代际令牌），旧 fetch 落地
//   不会再排 timer，所以隐藏回前台/手动刷新不会叠出两条并行轮询链；
// - 页面隐藏时不取数、也不排下一次，恢复交给 visibilitychange → reset；
// - stop（组件卸载）后清掉 timer，旧链 resolve 无任何副作用。
//
// options:
//   poll()                  取数一次，返回 Promise<boolean>（true=成功）
//   nextDelay(ok, prevMs)   根据本次结果与上一步等待算下一次等待
//   isHidden()              当前页面是否隐藏
//   setTimer(fn, ms)        定时器（浏览器 setTimeout，测试注入手动 timer 表）
//   clearTimer(id)          取消定时器
function createPollScheduler(options) {
  const poll = options.poll;
  const nextDelay = options.nextDelay;
  const isHidden = options.isHidden;
  const setTimer = options.setTimer;
  const clearTimer = options.clearTimer;

  let timer = null;
  let gen = 0;
  let stopped = false;
  let delay = 0;

  function clearPending() {
    if (timer === null) return;
    clearTimer(timer);
    timer = null;
  }

  function schedule(myGen, wait) {
    if (stopped || myGen !== gen) return;
    if (isHidden()) return;
    timer = setTimer(function () {
      timer = null;
      tick(myGen);
    }, wait);
  }

  function tick(myGen) {
    if (stopped || myGen !== gen) return;
    if (isHidden()) return;
    let request;
    try {
      request = poll();
    } catch (e) {
      request = Promise.reject(e);
    }
    Promise.resolve(request).then(
      function (ok) {
        if (stopped || myGen !== gen) return;
        delay = nextDelay(!!ok, delay);
        schedule(myGen, delay);
      },
      function () {
        if (stopped || myGen !== gen) return;
        delay = nextDelay(false, delay);
        schedule(myGen, delay);
      }
    );
  }

  // 重新开始一条新链：作废在途 fetch、清掉待定 timer、等待步长回到初始值。
  function reset() {
    if (stopped) return;
    gen += 1;
    clearPending();
    delay = 0;
    schedule(gen, 0);
  }

  function stop() {
    stopped = true;
    gen += 1;
    clearPending();
  }

  return { reset: reset, stop: stop };
}

export { createPollScheduler };
