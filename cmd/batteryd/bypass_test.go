package main

import "testing"

// 满电旁路放电：status=Full 但电量计在持续下降（充电器仍在、高负载取电）也要记。
// 方向只认电量计差分，不依赖 current_now 符号（内核可能充电报负）。开账要求连续
// ≥2 拍可信下降且累计 ≥5mAh，单拍回芯（真机满电充电期实测 -8/-48mAh）不算。
func TestDischargeBypassAtFull(t *testing.T) {
	r := newPipeRig(t)
	cc := int64(5_000_000)
	putCounter(t, r, cc)
	r.put(100, -1_000_000, 4_400_000)
	r.step("Full") // 首拍只建立探测基线
	if v, _ := r.st.KVGet(kvDisActive); v == "1" {
		t.Fatal("首拍只建立探测基线，不应开账")
	}
	// 15 拍、每拍 -16667µAh（≈1A×60s），cap 100→85；第 2 拍才满足连续下降条件。
	for i := 1; i <= 15; i++ {
		cc -= 16_667
		putCounter(t, r, cc)
		r.put(100-int64(i), -1_000_000, 4_400_000)
		r.step("Full")
	}
	if v, _ := r.st.KVGet(kvDisActive); v != "1" {
		t.Fatal("满电旁路放电应开账")
	}
	// 充电方向恢复：电量计单拍回升 ≥2mAh → 结算
	cc += 5_000
	putCounter(t, r, cc)
	r.put(85, 500_000, 4_300_000)
	r.step("Full")
	if v, _ := r.st.KVGet(kvDisActive); v == "1" {
		t.Fatal("充电方向恢复后应结算")
	}
	rows, err := r.st.RecentDischarge(5)
	if err != nil || len(rows) != 1 {
		t.Fatalf("应落一行放电记录, rows=%d err=%v", len(rows), err)
	}
	row := rows[0]
	// 探测拍 + 确认拍（第 2 拍下降）只定基线；其后 13 拍记入
	wantUah := int64(13 * 16_667)
	if row.UAh != wantUah {
		t.Fatalf("放出 = %d, want %d", row.UAh, wantUah)
	}
	if row.StartCap != 98 || row.EndCap != 85 {
		t.Fatalf("cap 98→85, got %d→%d", row.StartCap, row.EndCap)
	}
	if row.Implied == nil {
		t.Fatal("掉幅 13 应给隐含容量")
	}
}

// 单拍回芯/抖动（真机满电充电期出现过 -8/-48mAh 而电流仍充电）不能单独开账：
// 必须连续 2 拍下降；中间只要回升就把累计清零。
func TestDischargeBypassIgnoresSingleTickReset(t *testing.T) {
	r := newPipeRig(t)
	cc := int64(5_000_000)
	putCounter(t, r, cc)
	r.put(100, 1_000_000, 4_400_000)
	r.step("Full")
	for i := 0; i < 3; i++ {
		cc -= 20_000 // 单拍大跳（回芯），但下一拍回升
		putCounter(t, r, cc)
		r.put(100, 1_000_000, 4_400_000)
		r.step("Full")
		cc += 5_000
		putCounter(t, r, cc)
		r.put(100, 1_000_000, 4_400_000)
		r.step("Full")
	}
	if v, _ := r.st.KVGet(kvDisActive); v == "1" {
		t.Fatal("单拍回芯不应开账")
	}
	if n := countRows(t, r.st, "discharge"); n != 0 {
		t.Fatalf("不应有放电行, got %d", n)
	}
}

// 持续轻载下降同样要记账：单拍降幅小，但连续多拍累计到 5mAh 后开账。
func TestDischargeBypassSustainedLightDrain(t *testing.T) {
	r := newPipeRig(t)
	cc := int64(5_000_000)
	putCounter(t, r, cc)
	r.put(100, -100_000, 4_400_000)
	r.step("Full")
	for i := 0; i < 6; i++ {
		cc -= 1_000 // 1mAh/分钟 ≈60mA，持续 6 拍累计 6mAh
		putCounter(t, r, cc)
		r.put(100, -100_000, 4_400_000)
		r.step("Full")
	}
	if v, _ := r.st.KVGet(kvDisActive); v != "1" {
		t.Fatal("持续轻载下降累计到阈值后应开账")
	}
}

// 满电旁路延续到拔充后：同一会话继续累计，不应在 Full→Discharging 时断成两段。
func TestDischargeBypassContinuesAfterUnplug(t *testing.T) {
	r := newPipeRig(t)
	cc := int64(5_000_000)
	putCounter(t, r, cc)
	r.put(100, -1_000_000, 4_400_000)
	r.step("Full")
	for i := 1; i <= 10; i++ {
		cc -= 16_667
		putCounter(t, r, cc)
		r.put(100-int64(i), -1_000_000, 4_400_000)
		r.step("Full")
	}
	// 拔充：status 变 Discharging，同一会话继续
	for i := 1; i <= 3; i++ {
		cc -= 16_667
		putCounter(t, r, cc)
		r.put(90-int64(i), -1_000_000, 3_900_000)
		r.step("Discharging")
	}
	if v, _ := r.st.KVGet(kvDisActive); v != "1" {
		t.Fatal("拔充后应延续同一会话")
	}
	r.put(85, 3_000_000, 3_800_000)
	r.step("Charging")
	rows, err := r.st.RecentDischarge(5)
	if err != nil || len(rows) != 1 {
		t.Fatalf("应落一行, rows=%d err=%v", len(rows), err)
	}
	// Full 段：第 2 拍开账后 8 拍 + 放电段 3 拍
	if want := int64(11 * 16_667); rows[0].UAh != want {
		t.Fatalf("放出 = %d, want %d", rows[0].UAh, want)
	}
	if rows[0].StartCap != 98 {
		t.Fatalf("会话起点应在第 2 拍确认时 cap=98, got %d", rows[0].StartCap)
	}
}

// 满电旁路期间 cap 没掉（仍是 100）：结算时掉幅不足，只清会话不落行。
func TestDischargeBypassNoCapDropNoRow(t *testing.T) {
	r := newPipeRig(t)
	cc := int64(5_000_000)
	putCounter(t, r, cc)
	r.put(100, -1_000_000, 4_400_000)
	r.step("Full")
	for i := 0; i < 3; i++ {
		cc -= 16_667
		putCounter(t, r, cc)
		r.put(100, -1_000_000, 4_400_000)
		r.step("Full")
	}
	cc += 5_000
	putCounter(t, r, cc)
	r.put(100, 500_000, 4_400_000)
	r.step("Full") // 充电恢复，但 cap 100→100
	if v, _ := r.st.KVGet(kvDisActive); v == "1" {
		t.Fatal("应已结算")
	}
	if n := countRows(t, r.st, "discharge"); n != 0 {
		t.Fatalf("掉幅不足不应落行, got %d", n)
	}
}

// 满电旁路会话期间慢涓流回升（单拍增量 <2mAh）：连续 3 拍回升也应结算，
// 避免会话一直挂着、后续把充电增量从放电累计里扣掉。
func TestDischargeBypassSlowResume(t *testing.T) {
	r := newPipeRig(t)
	cc := int64(5_000_000)
	putCounter(t, r, cc)
	r.put(100, -1_000_000, 4_400_000)
	r.step("Full")
	// 首拍定基线，第 2 拍开账，第 3 拍再降一次
	for i := 0; i < 3; i++ {
		cc -= 16_667
		putCounter(t, r, cc)
		r.put(100, -1_000_000, 4_400_000)
		r.step("Full")
	}
	if v, _ := r.st.KVGet(kvDisActive); v != "1" {
		t.Fatal("应已开账")
	}
	// 慢涓流：每拍 +1000µAh（≈60mA），连续 3 拍回升后结算
	for i := 0; i < 3; i++ {
		cc += 1_000
		putCounter(t, r, cc)
		r.put(100, 100_000, 4_390_000)
		r.step("Full")
	}
	if v, _ := r.st.KVGet(kvDisActive); v == "1" {
		t.Fatal("连续 3 拍回升应结算")
	}
}
