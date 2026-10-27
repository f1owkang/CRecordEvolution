import { useState } from 'preact/hooks';
import { useTr } from '../ui/index.js';
import { useHistoryData } from './useHistoryData.js';
import { useHistoryFilters } from './useHistoryFilters.js';
import { HistoryControls } from './HistoryControls.jsx';
import { HistorySummary } from './HistorySummary.jsx';
import { TimelineList } from './TimelineList.jsx';
import { TrendsView } from './trends.jsx';

// 历史页内容：错误 / 骨架屏 / 记录视图 / 趋势视图四选一，History 只负责编排与状态。
function HistoryBody(props) {
  const { t, data, events, trends, loading, loadingMore, err, mode, filter } = props;
  if (err && !data) {
    return (
      <div class="hist-empty">
        <div class="t">{t('err_run', { detail: err.detail })}</div>
        <button type="button" class="load-more" onClick={() => props.load(true)}>
          {t('hist_retry')}
        </button>
      </div>
    );
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
  if (mode === 'trends') {
    return (
      <TrendsView trends={trends} openTile={props.openTile} onToggleTile={props.onToggleTile} />
    );
  }
  if (!events.length) {
    return (
      <div class="hist-empty">
        <div class="t">{filter === 'all' ? t('hist_empty_records') : t('hist_empty_filter')}</div>
        {filter !== 'all' ? (
          <button type="button" class="load-more" onClick={() => props.onFilter('all')}>
            {t('hist_filter_all')}
          </button>
        ) : null}
      </div>
    );
  }
  return (
    <TimelineList
      events={events}
      hasMore={data && data.has_more}
      loadingMore={loadingMore}
      openKey={props.openKey}
      onToggleEvent={props.onToggleEvent}
      onLoadMore={() => props.load(false)}
    />
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
