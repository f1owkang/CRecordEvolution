import { HistStat, useTr } from '../ui/index.js';
import { fmtInt } from '../core.js';

// 历史页摘要：有效会话 / 放电记录 / 采样数，以及有效会话的充入总量、
// 平均每次充入与平均时长；后三格的单位写在标题里，数值列只放纯数字。
function HistorySummary({ summary }) {
  const { t } = useTr();
  const avgMin = summary && summary.avg_secs != null ? Math.round(summary.avg_secs / 60) : null;
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
      <HistStat
        value={summary && summary.charge_mah != null ? fmtInt(summary.charge_mah) : '—'}
        label={t('history_summary_charge')}
      />
      <HistStat
        value={summary && summary.avg_mah != null ? fmtInt(summary.avg_mah) : '—'}
        label={t('history_summary_avg')}
      />
      <HistStat
        value={avgMin != null ? fmtInt(avgMin) : '—'}
        label={t('history_summary_duration')}
      />
    </section>
  );
}

export { HistorySummary };
