import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import {
  CMD_JSON,
  CMD_ONCE,
  POLL_BASE,
  POLL_CHARGE,
  POLL_MAX,
  chargeActive,
  contentFP,
  hasExec,
  run,
  runJsonDoc,
} from './core.js';
import { createPollScheduler } from './pollScheduler.js';

const HAS_HOST_EXEC = hasExec();

// 取数：取一份 json 快照，失败统一映射成 i18n 的 { key, vars }（key 沿用原字典），
// 并发请求做 in-flight 合并，并用 contentFP 内容指纹去重。
function useDocFetch() {
  const [doc, setDoc] = useState(null);
  const [err, setErr] = useState(null);
  const lastFP = useRef(null);
  // 最新一份成功解析的 doc（含被指纹去重跳过的那些）：轮询步长按它判定充电态。
  const lastDoc = useRef(null);
  const fetchInFlight = useRef(null);

  // 所有取数失败分支共用的收尾：置错误并返回 false。
  const fail = useCallback(function (error) {
    setErr(error);
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
      // errno/stdout/JSON/结构校验走 core 的共用解析，错误模型与历史页一致。
      const request = runJsonDoc(CMD_JSON).then(function (parsed) {
        if (parsed.error) return fail(parsed.error);
        const data = parsed.doc;
        lastDoc.current = data;
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
          return fail({ key: 'err_render', vars: { msg: String((e && e.message) || e) } });
        }
      );
      return fetchInFlight.current;
    },
    [fail]
  );

  return { doc, err, fetchDoc, lastDoc };
}

// 轮询：成功时按最新 doc 判定步长（充电中 15s，其余 60s），失败按 ×2 退避到 POLL_MAX；
// 页面隐藏时不取数也不排下一次，回到前台由 visibilitychange → reset 重启。
// resetRef 暴露 reset()，供手动刷新立即重排下一次轮询。
function usePolling(fetch, resetRef, lastDoc) {
  useEffect(
    function () {
      if (!HAS_HOST_EXEC) return undefined;
      const scheduler = createPollScheduler({
        poll: function () {
          return fetch(false);
        },
        // 每次取数后在「下一次」调度前重算步长：doc 变化（进入/退出充电）
        // 只要发生在这一轮，下一次等待就按新状态切换。
        nextDelay: function (ok, previousDelay) {
          if (ok) return chargeActive(lastDoc.current) ? POLL_CHARGE : POLL_BASE;
          return Math.min((previousDelay || POLL_BASE) * 2, POLL_MAX);
        },
        isHidden: function () {
          return !!document.hidden;
        },
        setTimer: function (fn, ms) {
          return setTimeout(fn, ms);
        },
        clearTimer: function (id) {
          clearTimeout(id);
        },
      });
      resetRef.current = function () {
        scheduler.reset();
      };
      scheduler.reset();
      function onVisibility() {
        if (!document.hidden) scheduler.reset();
      }
      document.addEventListener('visibilitychange', onVisibility);
      return function () {
        document.removeEventListener('visibilitychange', onVisibility);
        resetRef.current = null;
        scheduler.stop();
      };
    },
    [fetch, resetRef, lastDoc]
  );
}

// 概览/设置的数据来源：取数 hook + 退避轮询 hook + 手动刷新，本函数只做组合。
// 语义：document.hidden 短路、回到前台立即重置轮询、刷新后让历史页重载。
function useDocPolling() {
  const resetRef = useRef(null);
  const { doc, err, fetchDoc, lastDoc } = useDocFetch();
  usePolling(fetchDoc, resetRef, lastDoc);

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
