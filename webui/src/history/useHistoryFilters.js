import { useState } from 'preact/hooks';
import { HIST_DAYS_KEY, HIST_MODE_KEY, storedHistDays, storedHistMode } from '../core.js';

const HIST_MODES = ['records', 'trends'];
const HIST_FILTERS = ['all', 'charge', 'discharge'];
const HIST_DAY_RANGES = [7, 30, 90];

// 历史页筛选状态：视图模式与天数落 localStorage，充放电筛选只在本次会话内保留。
function useHistoryFilters() {
  const [mode, setMode] = useState(storedHistMode());
  const [filter, setFilter] = useState('all');
  const [days, setDays] = useState(storedHistDays());

  function changeMode(next) {
    if (!HIST_MODES.includes(next) || next === mode) return;
    setMode(next);
    try {
      localStorage.setItem(HIST_MODE_KEY, next);
    } catch (e) {}
  }

  function changeFilter(next) {
    if (!HIST_FILTERS.includes(next) || next === filter) return;
    setFilter(next);
  }

  function changeDays(next) {
    if (!HIST_DAY_RANGES.includes(next) || next === days) return;
    setDays(next);
    try {
      localStorage.setItem(HIST_DAYS_KEY, String(next));
    } catch (e) {}
  }

  return { mode, filter, days, changeMode, changeFilter, changeDays };
}

export { useHistoryFilters };
