import { useState } from 'preact/hooks';
import { useTr } from '../ui/index.js';
import { useHistoryData } from './useHistoryData.js';
import { useHistoryFilters } from './useHistoryFilters.js';
import { HistoryControls } from './HistoryControls.jsx';
import { HistorySummary } from './HistorySummary.jsx';
import { TimelineList } from './TimelineList.jsx';
import { TrendsView } from './trends.jsx';
import { historyErrorView, historyListBusy } from './historyView.js';

// 历史页错误行：文案沿用统一错误模型 t(err.key, err.vars)（与 ErrorBanner 同源），
// 样式复用历史页已有的错误空态；首屏失败与「已有数据但重载失败」共用同一组件。
// 重试按钮在重载在途时禁用，避免重复发起整页请求。
function HistoryErrorRow(props) {
  const { t, err, disabled, onRetry } = props;
  return (
    <div class="hist-empty" role="alert">
      <div class="t">{t(err.key, err.vars)}</div>
      <button type="button" class="load-more" disabled={disabled} onClick={onRetry}>
        {t('hist_retry')}
      </button>
    </div>
  );
}

// 历史页内容：错误 / 骨架屏 / 记录视图 / 趋势视图四选一，History 只负责编排与状态。
function HistoryBody(props) {
  const { t, data, events, trends, loading, loadingMore, err, mode, filter } = props;
  if (!data && err) {
    return <HistoryErrorRow t={t} err={err} disabled={loading} onRetry={() => props.load(true)} />;
  }
  if (!data && loading) {
    return (
      <div>
        <div class="skl skl-summary" />
        <div class="skl skl-row" />
        <div class="skl skl-row" />
        <div class="skl skl-chart" />
      </div>
    );
  }
  // 已有数据时错误行固定渲染在顶部：趋势视图与空列表下重载失败不再静默。
  // 「加载更早」失败同时保留列表底部的就近错误行（历史行为）。
  const flags = historyErrorView(err, data);
  const errorRow = flags.topError ? (
    <HistoryErrorRow
      t={t}
      err={err}
      disabled={loading}
      onRetry={() => props.load(flags.retryReset)}
    />
  ) : null;
  if (mode === 'trends') {
    return (
      <div>
        {errorRow}
        <TrendsView trends={trends} openTile={props.openTile} onToggleTile={props.onToggleTile} />
      </div>
    );
  }
  if (!events.length) {
    return (
      <div>
        {errorRow}
        <div class="hist-empty">
          <div class="t">{filter === 'all' ? t('hist_empty_records') : t('hist_empty_filter')}</div>
          {filter !== 'all' ? (
            <button type="button" class="load-more" onClick={() => props.onFilter('all')}>
              {t('hist_filter_all')}
            </button>
          ) : null}
        </div>
      </div>
    );
  }
  return (
    <div>
      {errorRow}
      <TimelineList
        events={events}
        hasMore={data && data.has_more}
        loadingMore={loadingMore}
        busy={historyListBusy(loading, err)}
        error={flags.pagingError}
        openKey={props.openKey}
        onToggleEvent={props.onToggleEvent}
        onLoadMore={() => props.load(false)}
        onRetry={() => props.load(!!(flags.pagingError && flags.pagingError.reset))}
      />
    </div>
  );
}

// 历史页：只负责组合「筛选 hook + 取数 hook + 三个展示块」。
function History({ active, reloadKey }) {
  const { t } = useTr();
  const { mode, filter, days, changeMode, changeFilter, changeDays } = useHistoryFilters();
  const [openKey, setOpenKey] = useState(null);
  const [openTile, setOpenTile] = useState(null);
  const { data, loading, loadingMore, err, load } = useHistoryData({
    active: active,
    reloadKey: reloadKey,
    days: days,
    filter: filter,
    mode: mode,
    onReset: () => setOpenKey(null),
  });
  const events = data && Array.isArray(data.events) ? data.events : [];
  const summary = data ? data.summary : null;
  const trends = data && data.trends ? data.trends : {};
  return (
    <section class={'page' + (active ? ' is-active' : '')} id="page_history" role="tabpanel">
      <HistorySummary summary={summary} />
      <HistoryControls
        mode={mode}
        days={days}
        filter={filter}
        onMode={changeMode}
        onDays={changeDays}
        onFilter={changeFilter}
      />
      <HistoryBody
        t={t}
        data={data}
        events={events}
        trends={trends}
        loading={loading}
        loadingMore={loadingMore}
        err={err}
        mode={mode}
        filter={filter}
        openKey={openKey}
        openTile={openTile}
        load={load}
        onFilter={changeFilter}
        onToggleEvent={setOpenKey}
        onToggleTile={setOpenTile}
      />
    </section>
  );
}

export { History };
