// 通道互检（数据质量哨兵）：把各路容量口径摆到一起互相印证。四路分别是
//   - ema：会话电流积分经 EMA 融合后的「实测」基线；
//   - mid：中段分窗隐含容量（电流积分 + 显示百分比折算）；
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

import "fmt"

const (
	// qualityMaxSpreadPct 互检偏差上限：最高与最低口径的相对偏差超过此值判不
	// 一致。实测正常设备四路互差 18%（充电积分口径略偏低），量纲误判期达 160%，
	// 取 40% 两不误伤。
	qualityMaxSpreadPct = 40
	// kvQualityOK 上一次互检结论（"1"/"0"）：只在结论翻转时落事件，避免刷屏
	kvQualityOK = "quality_ok"
)

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
func CapacityQuality(emaUA, midUA, disUA, fullUA int64) *Quality {
	names := [...]string{"ema", "mid", "dis", "full"}
	vals := [...]int64{emaUA, midUA, disUA, fullUA}
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

// checkQuality 通道互检装配：取各路口径比对，结论翻转时落事件留痕。任何失败
// 只静默，绝不影响结算主链路。
func (p *Pipeline) checkQuality() {
	q := CapacityQuality(
		kvInt(p.st, kvKeyEmaUA),
		kvInt(p.st, kvMidImpliedUA),
		kvInt(p.st, kvDisImpliedUA),
		p.fullUA,
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
