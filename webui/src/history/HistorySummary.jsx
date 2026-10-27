import { HistStat, useTr } from '../ui/index.js';
import { fmtInt } from '../core.js';

// 历史页摘要：有效会话 / 放电记录 / 采样数三张统计卡。
function HistorySummary({ summary }) {
  const { t } = useTr();
  return (
    <section class="hist-summary">
      <HistStat
        value={summary ? fmtInt(summary.valid_sessions) : '—'}
        label={t('history_summary_valid')}
      />
      <HistStat
        value={summary ? fmtInt(summary.discharges) : '—'}
        label={t('history_summary_discharges')}
      />
      <HistStat
        value={summary ? fmtInt(summary.samples) : '—'}
        label={t('history_summary_samples')}
      />
    </section>
  );
}

export { HistorySummary };
