import { Fragment } from 'preact';
import { Seg, useTr } from '../ui/index.js';

// 筛选 chip：范围与类型两行共用同一外观，选中态用 is-active + aria-pressed 表达。
function Chip({ active, onClick, children }) {
  return (
    <button
      type="button"
      class={'chip' + (active ? ' is-active' : '')}
      aria-pressed={active ? 'true' : 'false'}
      onClick={onClick}
    >
      {children}
    </button>
  );
}

const RANGE_DAYS = [7, 30, 90];
const RECORD_FILTERS = [
  ['all', 'hist_filter_all'],
  ['charge', 'hist_filter_charge'],
  ['discharge', 'hist_filter_discharge'],
];

// 历史页工具条：记录/趋势切换 + 时间范围 + （记录视图下）充放电筛选。
function HistoryControls({ mode, days, filter, onMode, onDays, onFilter }) {
  const { t } = useTr();
  return (
    <div class="hist-controls">
      <Seg
        id="hist_mode_seg"
        label={t('page_history')}
        value={mode}
        options={[
          { value: 'records', label: t('hist_mode_records') },
          { value: 'trends', label: t('hist_mode_trends') },
        ]}
        onChange={onMode}
      />
      <div class="hist-chips hist-chips--single" aria-label={t('hist_range_30')}>
        {RANGE_DAYS.map((dayCount) => (
          <Chip key={dayCount} active={days === dayCount} onClick={() => onDays(dayCount)}>
            {t('hist_range_' + dayCount)}
          </Chip>
        ))}
        {mode === 'records' ? (
          <Fragment>
            <span class="chip-sep" aria-hidden="true" />
            {RECORD_FILTERS.map(([value, labelKey]) => (
              <Chip key={value} active={filter === value} onClick={() => onFilter(value)}>
                {t(labelKey)}
              </Chip>
            ))}
          </Fragment>
        ) : null}
      </div>
    </div>
  );
}

export { HistoryControls };
