import { useTr } from '../ui/index.js';
import { plural } from '../i18n.js';
import { dayStart, fmtDayLabel } from '../core.js';
import { TimelineEvent } from './timeline.jsx';

// 事件按「显示日」分组；同一秒的多条事件用 kind:ts:序号 作为展开状态 key。
function groupEventsByDay(events) {
  const groups = [];
  const groupMap = new Map();
  events.forEach((event, index) => {
    const day = dayStart(event.ts);
    if (!groupMap.has(day)) {
      const group = { day: day, events: [] };
      groupMap.set(day, group);
      groups.push(group);
    }
    groupMap.get(day).events.push({ event: event, key: event.kind + ':' + event.ts + ':' + index });
  });
  return groups;
}

// 记录视图：按日期分组的时间轴 + 底部「加载更早 / 重试 / 已全部加载」。
// error 只在已有数据（分页失败）时由调用方传入：底部给一行错误 + 重试，
// 首屏失败仍走 History 顶部的空态错误样式。
function TimelineList({
  events,
  hasMore,
  loadingMore,
  error,
  openKey,
  onToggleEvent,
  onLoadMore,
  onRetry,
}) {
  const { t, lang } = useTr();
  const groups = groupEventsByDay(events);

  return (
    <div>
      <div class="timeline">
        {groups.map((group) => (
          <section class="tl-group" key={group.day}>
            <div class="tl-date">
              <span>{fmtDayLabel(lang, group.events[0].event.ts)}</span>
              <span class="tl-count">
                {plural(lang, group.events.length, 'hist_records_count_one', 'hist_records_count')}
              </span>
            </div>
            {group.events.map((item) => (
              <TimelineEvent
                key={item.key}
                event={item.event}
                kind={item.event.kind}
                open={openKey === item.key}
                onToggle={() => onToggleEvent(openKey === item.key ? null : item.key)}
              />
            ))}
          </section>
        ))}
      </div>
      <div class="hist-foot">
        {error ? (
          <div class="hist-more-err" role="alert">
            <div class="t">{t('err_run', { detail: error.detail })}</div>
            <button type="button" class="load-more" onClick={onRetry}>
              {t('hist_retry')}
            </button>
          </div>
        ) : hasMore ? (
          <button type="button" class="load-more" disabled={loadingMore} onClick={onLoadMore}>
            {loadingMore ? t('loading') : t('hist_load_more')}
          </button>
        ) : (
          <div class="note">{t('hist_all_loaded')}</div>
        )}
      </div>
    </div>
  );
}

export { TimelineList };
