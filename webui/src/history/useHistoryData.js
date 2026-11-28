import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import { run } from '../core.js';
import {
  buildHistoryCmd,
  createHistoryRequestTracker,
  historyBefore,
  mergeHistoryEvents,
  parseHistoryResult,
} from './historyCommand.js';

// 历史页取数：按「天数 / 筛选 / 视图」请求 history 子命令，支持游标加载更早记录。
// 返回的 load(reset) 由页面在首次激活、切筛选、点「加载更早」、点重试时调用。
//
// 竞态约束（见 historyCommand.createHistoryRequestTracker）：
// - 整页重载在途时忽略追加分页，避免旧游标页拼到新筛选结果上；
// - 响应落地必须同时匹配请求参数键 + 代际，迟到的旧响应直接丢弃；
// - 追加分页还要求当前数据属于当前参数键（切筛选但重载尚未落地的窗口期）。
function useHistoryData({ active, reloadKey, days, filter, mode, onReset }) {
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [err, setErr] = useState(null);
  const trackerRef = useRef(null);
  if (trackerRef.current == null) trackerRef.current = createHistoryRequestTracker();
  const tracker = trackerRef.current;
  const dataRef = useRef(null);
  // 上一次已应用的查询参数 / 外部刷新序号：用来区分「只是切回历史页」与真正的重载。
  const lastParams = useRef(null);
  const lastReload = useRef(reloadKey);
  const paramsKey = days + '|' + filter + '|' + mode;
  // 当前列表数据对应的参数键：切筛选后、重载落地前不允许拿旧游标追加。
  const dataKeyRef = useRef('');
  // 渲染时就同步参数键：响应落地校验要能看到最新参数，而不等 effect。
  tracker.setKey(paramsKey);

  useEffect(() => {
    dataRef.current = data;
  }, [data]);

  // 所有失败分支共用的收尾：置错误并复位两个 loading，避免逐处重复而漏掉其一。
  // reset 记录失败的是整页重载还是「加载更早」，供错误行决定重试哪种请求。
  const fail = useCallback((error, reset) => {
    setErr({ key: error.key, vars: error.vars, reset: !!reset });
    setLoading(false);
    setLoadingMore(false);
  }, []);

  const load = useCallback(
    (reset) => {
      if (!active) return;
      const previous = dataRef.current;
      let before = 0;
      if (!reset) {
        // 整页重载在途 / 上一次「加载更早」还没回包：直接忽略，避免并行两条链。
        if (tracker.isPendingReset() || tracker.isAppendInFlight()) return;
        if (dataKeyRef.current !== tracker.getKey()) return;
        before = historyBefore(previous);
        if (!before) return;
      }
      const token = tracker.begin(!!reset);
      const type = mode === 'records' ? filter : 'all';
      // 参数已过白名单/数值校验，拼进固定模板，不会透传任意字符串。
      const cmd = buildHistoryCmd({ days: days, type: type, before: before });
      if (reset) {
        setLoading(true);
        setErr(null);
        if (onReset) onReset();
      } else {
        setLoadingMore(true);
      }
      run(cmd).then(
        (result) => {
          // 过期响应（旧代际或旧参数键）：不落地、也不清掉新请求的在途状态。
          if (!tracker.settle(token)) return;
          const parsed = parseHistoryResult(result);
          if (parsed.error) {
            fail(parsed.error, reset);
            return;
          }
          const doc = parsed.doc;
          dataKeyRef.current = token.key;
          setData((old) => (reset ? doc : mergeHistoryEvents(old, doc)));
          setErr(null);
          setLoading(false);
          setLoadingMore(false);
        },
        (e) => {
          if (!tracker.settle(token)) return;
          fail({ key: 'err_render', vars: { msg: String((e && e.message) || e) } }, reset);
        }
      );
    },
    [active, days, filter, mode, tracker, fail, onReset]
  );

  // 只在首次激活、查询参数变化或外部刷新时重载：仅仅「切回历史页」（active 0→1）
  // 且已有数据时保留已加载的分页、展开详情与滚动位置，不重新拉第一页。
  useEffect(() => {
    if (!active) return;
    const paramsChanged = lastParams.current !== paramsKey;
    const reloaded = lastReload.current !== reloadKey;
    lastParams.current = paramsKey;
    lastReload.current = reloadKey;
    if (dataRef.current == null || paramsChanged || reloaded) load(true);
    // load 的依赖已覆盖 paramsKey/reloadKey；不把 load 列入依赖，避免它每次重建都触发重载。
  }, [active, reloadKey, paramsKey]);

  return { data, loading, loadingMore, err, load };
}

export { useHistoryData };
