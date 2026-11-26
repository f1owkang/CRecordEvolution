import { HistStat, useTr } from '../ui/index.js';
import { fmtInt } from '../core.js';

// 历史页摘要只保留三张关键卡：有效会话、累计充入、平均每次充入。
// 放电数/采样数/平均时长仍由 history 接口返回（诊断与其他视图可用），
// 但不再占据首屏摘要，避免六格把趋势内容往下挤。
function HistorySummary({ summary }) {
  const { t } = useTr();
  return (
    <section class="hist-summary">
      <HistStat
        value={summary ? fmtInt(summary.valid_sessions) : '—'}
        label={t('history_summary_valid')}
      />
      <HistStat
        value={summary && summary.charge_mah != null ? fmtInt(summary.charge_mah) : '—'}
        label={t('history_summary_charge')}
      />
      <HistStat
        value={summary && summary.avg_mah != null ? fmtInt(summary.avg_mah) : '—'}
        label={t('history_summary_avg')}
      />
    </section>
  );
}

export { HistorySummary };
