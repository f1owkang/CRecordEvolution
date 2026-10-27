import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import { CMD_JSON, CMD_ONCE, POLL_BASE, POLL_MAX, contentFP, hasExec, run } from './core.js';

const HAS_HOST_EXEC = hasExec();

// 概览/设置的数据来源：取一份 json 快照 + 自适应退避轮询 + 手动刷新。
// 保留 App 原有语义：in-flight 合并、contentFP 内容指纹去重、document.hidden 短路、
// 回到前台立即重置轮询、失败统一映射成 i18n 的 { key, vars }（key 沿用原字典）。
function useDocPolling() {
  const [doc, setDoc] = useState(null);
  const [err, setErr] = useState(null);
  const [busy, setBusy] = useState(false);
  const [histReload, setHistReload] = useState(0);
  const lastFP = useRef(null);
  const fetchInFlight = useRef(null);
  const pollCtl = useRef({ reset: null });

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
      const p = run(CMD_JSON).then(function (r) {
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
          setDoc(data);
          setErr(null);
          return true;
        }
        lastFP.current = fp;
        setDoc(data);
        setErr(null);
        return true;
      });
      fetchInFlight.current = p.then(
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

  // 轮询：成功固定 POLL_BASE，失败按 ×2 退避到 POLL_MAX；页面隐藏时 fetchDoc 直接跳过。
  useEffect(
    function () {
      if (!HAS_HOST_EXEC) return;
      let timer = null,
        delay = 0;
      function tick() {
        timer = setTimeout(function () {
          timer = null;
          fetchDoc(false).then(function (ok) {
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
      pollCtl.current.reset = reset;
      tick();
      function onVisibility() {
        if (!document.hidden) reset();
      }
      document.addEventListener('visibilitychange', onVisibility);
      return function () {
        if (timer) clearTimeout(timer);
        document.removeEventListener('visibilitychange', onVisibility);
        pollCtl.current.reset = null;
      };
    },
    [fetchDoc]
  );

  // 手动刷新：先跑一次 batteryd once，800ms 后强制取数、重置轮询并让历史页重载。
  function refreshNow() {
    if (busy) return;
    setBusy(true);
    run(CMD_ONCE).then(function () {
      setTimeout(function () {
        fetchDoc(true).then(function () {
          if (pollCtl.current.reset) pollCtl.current.reset();
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
