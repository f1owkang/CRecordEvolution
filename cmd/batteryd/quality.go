// 通道互检（数据质量哨兵）：把各路容量口径摆到一起互相印证。四路分别是
//   - ema：会话电流积分经 EMA 融合后的「实测」基线；
//   - mid：中段分窗隐含容量（电量计差分优先，无电量计读数才回退电流积分）；
//   - dis：放电差分隐含容量（charge_counter 芯片内积分 + 显示掉幅折算）；
//   - full：内核 charge_full（电量计自学习值）。
//
// 依据：四路同源于电池真实容量，正常设备存在系统性偏差但不大（实测同区间
// 配对下 ema 与内核口径差 ≤16%，dis 与内核差 0.8%），而量纲误判/脏样本会把
// 电流积分通道整体放大数倍——实测中段口径一度达内核口径的 258%、单会话隐含
// 达设计容量的 1.5 倍，且单看任一路都察觉不了，只有互检能当天暴露（该回归
// 实际静默了 11 天）。
//
// 纪律与 ica.go / midwin.go 一致：CapacityQuality 为无 IO 纯函数；装配（取 kv、
// 事件留痕、出口字段）分别在 Pipeline.checkQuality 与 jsonout 完成。

package main

import (
	"fmt"
	"strconv"
)

const (
	// qualityMaxSpreadPct 互检偏差上限：最高与最低口径的相对偏差超过此值判不
	// 一致。实测正常设备四路互差 18%（充电积分口径略偏低），量纲误判期达 160%，
	// 取 40% 两不误伤。
	qualityMaxSpreadPct = 40
	// kvQualityOK 上一次互检结论（"1"/"0"）：只在结论翻转时落事件，避免刷屏
	kvQualityOK = "quality_ok"
	// kvMidImpliedTs / kvDisImpliedTs 对应隐含容量的最近更新时间（Unix 秒）：
	// 互检前按 retainDays 保留窗口做时效判定，过期渠道按缺失处理，避免半年没
	// 放电的设备拿旧值永久误报/掩盖问题。写隐含值时与值一起落盘。
	kvMidImpliedTs = "mid_implied_ts"
	kvDisImpliedTs = "dis_implied_ts"
)

// impliedFreshness 为一路隐含容量口径的时效判定结果。
type impliedFreshness struct {
	// ValueUAh 参与互检的值（µAh）：过期渠道为 0（按缺失处理）。
	ValueUAh int64
	// AgeSecs 距最近更新的秒数；ts 缺失或非法时为 0。
	AgeSecs int64
	// Stale 值存在但已超过保留窗口，不参与互检。
	Stale bool
	// MissingTS 值存在但缺时间戳（旧库升级）：不判死，按未过期处理，
	// 由装配层回填当前时间给一次宽限。
	MissingTS bool
}

// judgeImpliedFreshness 判断某路隐含容量口径是否仍在保留窗口内。retainDays
// 复用数据保留窗口（main.go 的 retainDays），不另设时效 magic number：
// 超过 retainDays×86400 秒未更新即视为过期。valueUAh<=0 表示该路本就缺失；
// ts<=0 表示旧库无时间戳，按「未过期 + 需回填宽限」处理；时钟回拨（ts 在未来）
// 不判过期。
func judgeImpliedFreshness(valueUAh, updatedTs, now int64, retainDays int) impliedFreshness {
	f := impliedFreshness{ValueUAh: valueUAh}
	if valueUAh <= 0 {
		return f
	}
	if updatedTs <= 0 {
		f.MissingTS = true
		return f
	}
	if age := now - updatedTs; age > 0 {
		f.AgeSecs = age
	}
	if maxAge := int64(retainDays) * 86400; f.AgeSecs > maxAge {
		f.Stale = true
		f.ValueUAh = 0
	}
	return f
}

// freshImpliedUAh 只读地读取一路隐含容量并按保留窗口过滤，供 stats() 的
// JSON 出口使用（不写库、不落事件）：概览的「口径一致/互差」必须与结算侧
// checkQuality 用同一份时效口径，否则旧值仍会在概览里误报。
func freshImpliedUAh(st KVStore, valueKey, tsKey string, now int64) int64 {
	return judgeImpliedFreshness(kvInt(st, valueKey), kvInt(st, tsKey), now, retainDays).ValueUAh
}

// Quality 通道互检结果。各路单位 mAh，缺哪路省哪路（出口按「缺失即省略」降级）。
type Quality struct {
	OK        bool   `json:"ok"`
	SpreadPct int64  `json:"spread_pct"`
	High      string `json:"high,omitempty"`
	Low       string `json:"low,omitempty"`
	EmaMah    *int64 `json:"ema_mah,omitempty"`
	MidMah    *int64 `json:"mid_mah,omitempty"`
	DisMah    *int64 `json:"dis_mah,omitempty"`
	FullMah   *int64 `json:"full_mah,omitempty"`
}

// CapacityQuality 比较各路容量口径（单位 µAh）的相互偏差。可用路数 < 2 时
// 返回 nil（无从互检）；各路 ≤0 视为缺失，不参与比较。
func CapacityQuality(emaUAh, midUAh, disUAh, fullUAh int64) *Quality {
	names := [...]string{"ema", "mid", "dis", "full"}
	vals := [...]int64{emaUAh, midUAh, disUAh, fullUAh}
	q := Quality{OK: true}
	var slots [4]*int64
	lo, hi := int64(0), int64(0)
	loName, hiName := "", ""
	n := 0
	for i, v := range vals {
		if v <= 0 {
			continue
		}
		n++
		mah := v / 1000
		slots[i] = &mah
		if lo == 0 || v < lo {
			lo, loName = v, names[i]
		}
		if v > hi {
			hi, hiName = v, names[i]
		}
	}
	if n < 2 {
		return nil
	}
	q.EmaMah, q.MidMah, q.DisMah, q.FullMah = slots[0], slots[1], slots[2], slots[3]
	q.High, q.Low = hiName, loName
	q.SpreadPct = (hi - lo) * 100 / lo
	q.OK = q.SpreadPct <= qualityMaxSpreadPct
	return &q
}

// checkQuality 通道互检装配：取各路口径比对，结论翻转时落事件留痕。mid/dis
// 两路隐含容量先过时效判定：超过保留窗口的按缺失处理（不参与互检）并落一次
// quality_stale 事件；旧库缺时间戳时回填当前时间给一次宽限。任何失败只静默，
// 绝不影响结算主链路。
func (p *Pipeline) checkQuality() {
	now := p.now().Unix()
	q := CapacityQuality(
		kvInt(p.st, kvKeyEmaUAh),
		p.impliedForQuality(kvMidImpliedUAh, kvMidImpliedTs, "mid", now),
		p.impliedForQuality(kvDisImpliedUAh, kvDisImpliedTs, "dis", now),
		p.fullUAh,
	)
	if q == nil {
		return
	}
	cur := "1"
	if !q.OK {
		cur = "0"
	}
	if prev, ok := p.st.KVGet(kvQualityOK); ok && prev == cur {
		return
	}
	_ = p.st.KVSet(kvQualityOK, cur)
	if q.OK {
		p.log("[互检] 四路容量口径一致（最大互差 %d%%）", q.SpreadPct)
		return
	}
	p.log("[互检] 容量口径互差 %d%% 超限（%s 偏高 / %s 偏低），已留痕",
		q.SpreadPct, q.High, q.Low)
	_ = p.st.InsertEvent("quality_warn",
		fmt.Sprintf("spread=%d%% high=%s low=%s", q.SpreadPct, q.High, q.Low))
}

// impliedForQuality 读取一路隐含容量并做时效装配：过期的按缺失（0）返回并落
// quality_stale 事件；缺时间戳的旧库回填当前时间（宽限 retainDays 天）并说明。
// 值本身缺失（≤0）时什么都不做。
func (p *Pipeline) impliedForQuality(valueKey, tsKey, name string, now int64) int64 {
	f := judgeImpliedFreshness(kvInt(p.st, valueKey), kvInt(p.st, tsKey), now, retainDays)
	switch {
	case f.MissingTS:
		if err := p.st.KVSet(tsKey, strconv.FormatInt(now, 10)); err != nil {
			_ = p.st.InsertEvent("quality_stale", fmt.Sprintf(
				"%s 缺少时间戳且回填失败：%v（本次仍参与互检）", name, err))
		} else {
			_ = p.st.InsertEvent("quality_stale", fmt.Sprintf(
				"%s 缺少时间戳，已按当前时间回填宽限 %d 天", name, retainDays))
		}
	case f.Stale:
		_ = p.st.InsertEvent("quality_stale", fmt.Sprintf(
			"%s 已 %d 天未更新，不参与互检", name, f.AgeSecs/86400))
	}
	return f.ValueUAh
}
