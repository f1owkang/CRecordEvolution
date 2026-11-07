import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import { CMD_HISTORY, HIST_PAGE_LIMIT, run } from '../core.js';

// 追加分页时按「kind:ts」去重：游标边界可能返回与上一页重叠的事件。
function mergeEvents(previous, next) {
  if (!previous) return next;
  const seen = new Set();
  const merged = [];
  previous.events.concat(next.events).forEach((event) => {
    const key = event.kind + ':' + event.ts;
    if (seen.has(key)) return;
    seen.add(key);
    merged.push(event);
  });
  return {
    events: merged,
    has_more: next.has_more,
    next_before: next.next_before,
    summary: next.summary,
    trends: next.trends,
  };
}

// 历史页取数：按「天数 / 筛选 / 视图」请求 history 子命令，支持游标加载更早记录。
// 返回的 load(reset) 由页面在首次激活、切筛选、点「加载更早」、点重试时调用。
function useHistoryData({ active, reloadKey, days, filter, mode, onReset }) {
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [err, setErr] = useState(null);
  const requestSeq = useRef(0);
  const dataRef = useRef(null);
  // 上一次已应用的查询参数 / 外部刷新序号：用来区分「只是切回历史页」与真正的重载。
  const lastParams = useRef(null);
  const lastReload = useRef(reloadKey);
  const paramsKey = days + '|' + filter + '|' + mode;

  useEffect(() => {
    dataRef.current = data;
  }, [data]);

  // 所有失败分支共用的收尾：置错误并复位两个 loading，避免逐处重复而漏掉其一。
  // reset 记录失败的是整页重载还是「加载更早」，供底部错误行决定重试哪种请求。
  const fail = useCallback((detail, reset) => {
    setErr({ detail, reset: !!reset });
    setLoading(false);
    setLoadingMore(false);
  }, []);

  const load = useCallback(
    (reset) => {
      if (!active) return;
      const previous = dataRef.current;
      const before = reset ? 0 : previous && previous.has_more ? previous.next_before : 0;
      if (!reset && !before) return;
      // seq 递增用于丢弃过期响应：快速切筛选时先发的请求可能后到。
      const seq = ++requestSeq.current;
      const type = mode === 'records' ? filter : 'all';
      if (reset) {
        setLoading(true);
        setErr(null);
        if (onReset) onReset();
      } else {
        setLoadingMore(true);
      }
      let cmd = CMD_HISTORY + ' --days ' + days + ' --limit ' + HIST_PAGE_LIMIT + ' --type ' + type;
      if (before) cmd += ' --before ' + before;
      run(cmd).then(
        (result) => {
          if (seq !== requestSeq.current) return;
          if (result.errno !== 0) {
            return fail(result.stderr || 'errno ' + result.errno, reset);
          }
          const stdout = String(result.stdout || '').trim();
          if (!stdout) return fail('empty', reset);
          let doc;
          try {
            doc = JSON.parse(stdout);
          } catch (e) {
            return fail('json', reset);
          }
          if (!doc || !Array.isArray(doc.events)) return fail('struct', reset);
          setData((old) => (reset ? doc : mergeEvents(old, doc)));
          setErr(null);
          setLoading(false);
          setLoadingMore(false);
        },
        (e) => {
          if (seq !== requestSeq.current) return;
          fail(String((e && e.message) || e), reset);
        }
      );
    },
    [active, days, filter, mode, fail, onReset]
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
