import { useState } from 'preact/hooks';
import { useTr } from '../ui/index.js';
import { HIST_DAYS_KEY, HIST_MODE_KEY, storedHistDays, storedHistMode } from '../core.js';
import { useHistoryData } from './useHistoryData.js';
import { HistoryControls } from './HistoryControls.jsx';
import { HistorySummary } from './HistorySummary.jsx';
import { TimelineList } from './TimelineList.jsx';
import { TrendsView } from './trends.jsx';

// 历史页：只负责编排「取数 hook + 三个展示块」。取数与分页在 useHistoryData，
// 记录视图在 TimelineList，趋势视图在 TrendsView。
function History({ active, reloadKey }) {
  const { t } = useTr();
  const [mode, setMode] = useState(storedHistMode());
  const [filter, setFilter] = useState('all');
  const [days, setDays] = useState(storedHistDays());
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

  function changeMode(next) {
    if (next !== 'records' && next !== 'trends') return;
    if (next === mode) return;
    setMode(next);
    try {
      localStorage.setItem(HIST_MODE_KEY, next);
    } catch (e) {}
  }

  function changeFilter(next) {
    if (next !== 'all' && next !== 'charge' && next !== 'discharge') return;
    if (next !== filter) setFilter(next);
  }

  function changeDays(next) {
    if (next !== 7 && next !== 30 && next !== 90) return;
    if (next === days) return;
    setDays(next);
    try {
      localStorage.setItem(HIST_DAYS_KEY, String(next));
    } catch (e) {}
  }

  let body;
  if (err && !data) {
    body = (
      <div class="hist-empty">
        <div class="t">{t('err_run', { detail: err.detail })}</div>
        <button type="button" class="load-more" onClick={() => load(true)}>
          {t('hist_retry')}
        </button>
      </div>
    );
  } else if (!data && loading) {
    body = (
      <div>
        <div class="skl skl-summary" />
        <div class="skl skl-row" />
        <div class="skl skl-row" />
        <div class="skl skl-chart" />
      </div>
    );
  } else if (mode === 'records') {
    body = events.length ? (
      <TimelineList
        events={events}
        hasMore={data && data.has_more}
        loadingMore={loadingMore}
        openKey={openKey}
        onToggleEvent={setOpenKey}
        onLoadMore={() => load(false)}
      />
    ) : (
      <div class="hist-empty">
        <div class="t">{filter === 'all' ? t('hist_empty_records') : t('hist_empty_filter')}</div>
        {filter !== 'all' ? (
          <button type="button" class="load-more" onClick={() => changeFilter('all')}>
            {t('hist_filter_all')}
          </button>
        ) : null}
      </div>
    );
  } else {
    body = <TrendsView trends={trends} openTile={openTile} onToggleTile={setOpenTile} />;
  }

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
      {body}
    </section>
  );
}

export { History };
