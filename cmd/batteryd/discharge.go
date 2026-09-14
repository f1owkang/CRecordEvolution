// 放电会话记录（初步）：读电量计硬件库仑计 charge_counter（µAh）做差分累计，
// 转入充电时结算一条放电会话。与 current_now 积分不同，差分口径的采样在
// 芯片内完成，不受负载电流毫秒级突变混叠影响——这也是 AccuBattery 类应用
// 的取数路径（本机已验证节点存在）。
//
// 已知取舍（初步版）：
//   - 只在 status=Discharging 的拍累计：旁路供电机型（status=Full 但电池
//     实际放电）会漏记，留待后续按带符号电流补判；
//   - 电量计回修（差分反向或单跳 > disResyncUAh）只重置基线不计数；
//   - 隐含容量 = 放出电量×100÷显示掉幅，掉幅 ≥ disMinEstCap 才给出，
//     其余行仅作使用量记录，不参与任何估算通道。

package main

import (
	"strconv"
	"time"
)

const (
	kvDisActive   = "dis_active"
	kvDisStartTs  = "dis_start_ts"
	kvDisStartCap = "dis_start_cap"
	kvDisLastCC   = "dis_last_cc" // 0 = 重启后待重定基线哨兵
	kvDisAcc      = "dis_acc_uah"

	disResyncUAh = 100_000 // 单次差分超 100mAh 判电量计回修，只重置基线
	disMinRowCap = 5       // 显示掉幅 <5 个百分点不落行
	disMinEstCap = 10      // 显示掉幅 <10 个百分点不给隐含容量
)

// disState 放电会话状态；active 取 0/1 便于 kv 持久化。
type disState struct {
	active   int64
	startTs  int64
	startCap int64
	lastCC   int64
	accUAh   int64
}

// trackDischarge 每 Tick 首位调用：Discharging 差分累计，Charging 触发结算，
// 其余状态保持会话等待（拔充后继续同一会话）。
func (p *Pipeline) trackDischarge(status string) {
	if p.dis.active == 1 && status == "Charging" {
		p.settleDischarge()
		return
	}
	if status != "Discharging" {
		return
	}
	// charge_counter 缺失时一次性禁用：nodePath 对 miss 不做负缓存，
	// 否则每拍都全树扫 /sys（AGENTS 已知局限的hot路径版）
	if p.disCCOff {
		return
	}
	cc, err := p.readNode("charge_counter")
	if err != nil {
		p.disCCOff = true
		p.log("[放电] charge_counter 节点缺失，放电记录停用")
		return
	}
	capVal, err := p.readNode("capacity")
	if err != nil {
		return
	}
	now := p.now().Unix()
	if p.dis.active != 1 {
		p.dis = disState{active: 1, startTs: now, startCap: capVal, lastCC: cc}
		p.persistDis()
		p.log("[放电] 会话开始 cap=%d%%", capVal)
		return
	}
	if p.dis.lastCC == 0 { // 重启后续记：首拍只重定基线，不跨死亡间隙计数
		p.dis.lastCC = cc
		p.persistDis()
		return
	}
	if delta := p.dis.lastCC - cc; delta >= 0 && delta <= disResyncUAh {
		p.dis.accUAh += delta
	} // 反向或超限跳变：电量计回修，重置基线不计数
	p.dis.lastCC = cc
	p.persistDis()
}

// settleDischarge 充电插入即结算：掉幅达 disMinRowCap 落一行 discharge 表。
func (p *Pipeline) settleDischarge() {
	s := p.dis
	endCap := s.startCap
	if v, err := p.readNode("capacity"); err == nil {
		endCap = v
	}
	dur := p.now().Unix() - s.startTs
	drop := s.startCap - endCap
	p.clearDis()
	if drop < disMinRowCap || s.accUAh <= 0 {
		return // 太短不落行，静默
	}
	row := DischargeRow{TS: p.now().Unix(), Secs: dur, Uah: s.accUAh,
		StartCap: s.startCap, EndCap: endCap}
	durTxt := (time.Duration(dur) * time.Second).Truncate(time.Second)
	if drop >= disMinEstCap {
		implied := s.accUAh * 100 / drop
		row.Implied = &implied
		p.log("[放电] 结算 cap=%d→%d dur=%s 放出=%dmAh 隐含≈%dmAh",
			s.startCap, endCap, durTxt, s.accUAh/1000, implied/1000)
	} else {
		row.InvalidReason = "drop_lt_10"
		p.log("[放电] 结算 cap=%d→%d dur=%s 放出=%dmAh（掉幅不足，不给隐含）",
			s.startCap, endCap, durTxt, s.accUAh/1000)
	}
	if err := p.st.InsertDischarge(row); err != nil {
		_ = p.st.InsertEvent("discharge_fail", err.Error())
	}
}

// restoreDischarge 从 kv 恢复放电会话（daemon 重启续记）。lastCC 强制清零
// 走重定基线哨兵：进程死亡期间的差分无法区分「合法放电」与「电量计回修」，
// 一律不跨间隙计数（已累计的 accUAh 保留）。
func (p *Pipeline) restoreDischarge() {
	if v, ok := p.st.KVGet(kvDisActive); !ok || v != "1" {
		return
	}
	p.dis = disState{
		active:   1,
		startTs:  kvInt(p.st, kvDisStartTs),
		startCap: kvInt(p.st, kvDisStartCap),
		lastCC:   0,
		accUAh:   kvInt(p.st, kvDisAcc),
	}
}

func (p *Pipeline) persistDis() {
	_ = p.st.KVSet(kvDisActive, "1")
	for _, kv := range []struct {
		k string
		v int64
	}{
		{kvDisStartTs, p.dis.startTs},
		{kvDisStartCap, p.dis.startCap},
		{kvDisLastCC, p.dis.lastCC},
		{kvDisAcc, p.dis.accUAh},
	} {
		_ = p.st.KVSet(kv.k, strconv.FormatInt(kv.v, 10))
	}
}

func (p *Pipeline) clearDis() {
	p.dis = disState{}
	for _, k := range []string{kvDisActive, kvDisStartTs, kvDisStartCap, kvDisLastCC, kvDisAcc} {
		_ = p.st.KVSet(k, "0")
	}
}
