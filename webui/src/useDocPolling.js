import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import { CMD_JSON, CMD_ONCE, POLL_BASE, POLL_MAX, contentFP, hasExec, run } from './core.js';

const HAS_HOST_EXEC = hasExec();

// 取数：取一份 json 快照，失败统一映射成 i18n 的 { key, vars }（key 沿用原字典），
// 并发请求做 in-flight 合并，并用 contentFP 内容指纹去重。
function useDocFetch() {
  const [doc, setDoc] = useState(null);
  const [err, setErr] = useState(null);
  const lastFP = useRef(null);
  const fetchInFlight = useRef(null);

  // 所有取数失败分支共用的收尾：置错误并返回 false。
  const fail = useCallback(function (key, vars) {
    setErr({ key, vars });
    return false;
  }, []);

  const fetchDoc = useCallback(
    function (force) {
      if (document.hidden) return Promise.resolve(true);
      if (fetchInFlight.current) {
        if (!force) return fetchInFlight.current;
        return fetchInFlight.current.then(function () {
          return fetchDoc(true);
        });
      }
      const request = run(CMD_JSON).then(function (r) {
        if (r.errno !== 0) return fail('err_run', { detail: r.stderr || 'errno ' + r.errno });
        const stdout = String(r.stdout || '').trim();
        if (!stdout) return fail('err_noout');
        let data;
        try {
          data = JSON.parse(stdout);
        } catch (e) {
          return fail('err_json');
        }
        if (!data || typeof data !== 'object') return fail('err_struct');
        const fp = contentFP(data);
        if (!force && fp === lastFP.current) {
          // 内容未变 → 不写 state（setDoc 会塞入新对象，等于没去重），避免整树重渲染；
          // 只兜底清掉上一轮错误。
          setErr(null);
          return true;
        }
        lastFP.current = fp;
        setDoc(data);
        setErr(null);
        return true;
      });
      fetchInFlight.current = request.then(
        function (ok) {
          fetchInFlight.current = null;
          return ok;
        },
        function (e) {
          fetchInFlight.current = null;
          return fail('err_render', { msg: String((e && e.message) || e) });
        }
      );
      return fetchInFlight.current;
    },
    [fail]
  );

  return { doc, err, fetchDoc };
}

// 轮询：成功固定 POLL_BASE，失败按 ×2 退避到 POLL_MAX；页面隐藏时 fetchDoc 直接跳过。
// resetRef 暴露 reset()，供手动刷新立即重排下一次轮询。
function usePolling(fetch, resetRef) {
  useEffect(
    function () {
      if (!HAS_HOST_EXEC) return;
      let timer = null;
      let delay = 0;
      function tick() {
        timer = setTimeout(function () {
          timer = null;
          fetch(false).then(function (ok) {
            delay = ok ? POLL_BASE : Math.min((delay || POLL_BASE) * 2, POLL_MAX);
            tick();
          });
        }, delay);
      }
      function reset() {
        if (timer) {
          clearTimeout(timer);
          timer = null;
        }
        delay = 0;
        tick();
      }
      resetRef.current = reset;
      tick();
      function onVisibility() {
        if (!document.hidden) reset();
      }
      document.addEventListener('visibilitychange', onVisibility);
      return function () {
        if (timer) clearTimeout(timer);
        document.removeEventListener('visibilitychange', onVisibility);
        resetRef.current = null;
      };
    },
    [fetch, resetRef]
  );
}

// 概览/设置的数据来源：取数 hook + 退避轮询 hook + 手动刷新，本函数只做组合。
// 保留 App 原有语义：document.hidden 短路、回到前台立即重置轮询、刷新后让历史页重载。
function useDocPolling() {
  const resetRef = useRef(null);
  const { doc, err, fetchDoc } = useDocFetch();
  usePolling(fetchDoc, resetRef);

  const [busy, setBusy] = useState(false);
  const [histReload, setHistReload] = useState(0);

  // 手动刷新：先跑一次 batteryd once，800ms 后强制取数、重置轮询并让历史页重载。
  function refreshNow() {
    if (busy) return;
    setBusy(true);
    run(CMD_ONCE).then(function () {
      setTimeout(function () {
        fetchDoc(true).then(function () {
          if (resetRef.current) resetRef.current();
          setHistReload(function (n) {
            return n + 1;
          });
          setBusy(false);
        });
      }, 800);
    });
  }

  return { doc, err, busy, histReload, refreshNow };
}

export { useDocPolling };
