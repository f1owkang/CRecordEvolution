// 历史页错误展示与重试语义（纯函数，供 History.jsx 与单测共用）：
// - 还没有数据：首屏失败，用空态错误样式；
// - 已有数据：错误行固定渲染在 HistoryBody 顶部，记录与趋势两种视图共用
//   （此前趋势视图与空列表下的重载失败完全静默）；
// - 「加载更早」失败（err.reset=false）同时保留列表底部的就近错误行（历史行为）；
// - err.reset 决定重试整页（load(true)）还是同一游标页（load(false)）。
function historyErrorView(err, data) {
  const hasError = !!err;
  const hasData = !!data;
  return {
    emptyError: hasError && !hasData,
    topError: hasError && hasData,
    pagingError: hasError && hasData && !err.reset ? err : null,
    retryReset: !!(err && err.reset),
  };
}

// 重载在途 / 上一轮重载失败时不要留下可交互的旧列表：禁用「加载更早」与重试按钮。
function historyListBusy(loading, err) {
  return !!loading || !!(err && err.reset);
}

export { historyErrorView, historyListBusy };
