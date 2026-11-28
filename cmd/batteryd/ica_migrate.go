package main

import "fmt"

// ICA 相对峰高基准的跨版本迁移。
//
// 旧实现把样本里「多个恒流段拼接处」的长间隙也按 UA×gap 计入到达电压桶：真机
// 回放中同一会话的主峰可被放大近 10 倍。ica_peak_base 是首笔合格会话的绝对
// 峰高，一旦在旧口径下写入就带着虚高的量级；新实现只累计段内 dQ，若沿用旧
// 基准，升级后所有会话的 peak_h_rel 会掉到 ~0.1，趋势图出现断崖。
//
// 迁移策略：升级后第一次启动删除旧基准，让下一笔合格会话用新口径重建
// （rel 仍以 1 为基准，历史 ica_peaks 的相对值保持原量级），并落一次事件。
// 标记使用独立 kv 键，重复执行幂等；删除失败不落标记，下次再试。
const (
	kvICAPeakBaseSchema = "ica_peak_base_schema"
	icaPeakBaseSchema   = "2"
)

// migrateICAPeakBase 由所有命令入口的 newApp 调用（幂等、一次 kv 读短路）。
func migrateICAPeakBase(st *Store) {
	if v, ok := st.KVGet(kvICAPeakBaseSchema); ok && v == icaPeakBaseSchema {
		return
	}
	if old, has := st.KVGet(kvICAPeakBase); has {
		if err := st.KVDel(kvICAPeakBase); err != nil {
			return
		}
		_ = st.InsertEvent("ica_rebase", fmt.Sprintf("ICA 基准按段内口径重建（旧 base=%s）", old))
	}
	_ = st.KVSet(kvICAPeakBaseSchema, icaPeakBaseSchema)
}
