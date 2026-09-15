package main

import (
	"path/filepath"
	"testing"
	"time"
)

func putCC(t *testing.T, r *pipeRig, uah int64) {
	t.Helper()
	writeFile(t, filepath.Join(r.fs.Base, "battery", "charge_counter"), fmtNode(uah))
}

// 正常放电会话：cc 匀速下降、电量同步回落，插入充电后结算一行并给出隐含容量。
func TestDischargeSessionSettlesOnCharge(t *testing.T) {
	r := newPipeRig(t)
	cc := int64(5_000_000)
	// 15 拍放电：每拍 -8333µAh（≈0.5A×60s），cap 80→65
	for i := 0; i < 15; i++ {
		cc -= 8333
		putCC(t, r, cc)
		r.put(80-int64(i), -500_000, 3_800_000)
		r.step("Discharging")
	}
	if v, _ := r.st.KVGet(kvDisActive); v != "1" {
		t.Fatal("放电期间会话应活跃")
	}
	r.put(65, 3_000_000, 3_700_000)
	r.step("Charging")
	if v, _ := r.st.KVGet(kvDisActive); v == "1" {
		t.Fatal("充电插入后放电会话应结算")
	}
	rows, err := r.st.RecentDischarge(5)
	if err != nil || len(rows) != 1 {
		t.Fatalf("应落一行放电记录, rows=%d err=%v", len(rows), err)
	}
	row := rows[0]
	wantUah := int64(14 * 8333) // 首拍只定基线，其后 14 拍差分
	if row.Uah != wantUah {
		t.Fatalf("放出 = %d, want %d", row.Uah, wantUah)
	}
	if row.StartCap != 80 || row.EndCap != 65 {
		t.Fatalf("cap 80→65, got %d→%d", row.StartCap, row.EndCap)
	}
	if row.Implied == nil {
		t.Fatal("掉幅 15 应给隐含容量")
	}
	if want := wantUah * 100 / 15; *row.Implied != want {
		t.Fatalf("隐含 = %d, want %d", *row.Implied, want)
	}
}

// 电量计回修：单跳超 100mAh 只重置基线，不计入放出量。
func TestDischargeResyncSkipped(t *testing.T) {
	r := newPipeRig(t)
	putCC(t, r, 5_000_000)
	r.put(80, -500_000, 3_800_000)
	r.step("Discharging")  // 定基线 5_000_000
	putCC(t, r, 4_900_000) // 单跳 -100000，恰在界内会计入
	r.put(79, -500_000, 3_790_000)
	r.step("Discharging")
	putCC(t, r, 4_300_000) // 单跳 -600000 超界，判回修不计数
	r.put(78, -500_000, 3_780_000)
	r.step("Discharging")
	putCC(t, r, 4_291_667) // 正常 -8333
	r.put(77, -500_000, 3_770_000)
	r.step("Discharging")
	r.put(65, 3_000_000, 3_700_000)
	r.step("Charging")
	rows, _ := r.st.RecentDischarge(5)
	if len(rows) != 1 {
		t.Fatalf("应落一行, got %d", len(rows))
	}
	if want := int64(100_000 + 8333); rows[0].Uah != want {
		t.Fatalf("回修跳变不应计入: uah=%d want=%d", rows[0].Uah, want)
	}
}

// 重启续记：跨死亡间隙的大差分不计数（重定基线），已累计值保留。
func TestDischargeRestartRebasesBaseline(t *testing.T) {
	r := newPipeRig(t)
	putCC(t, r, 5_000_000)
	r.put(80, -500_000, 3_800_000)
	r.step("Discharging")
	putCC(t, r, 4_991_667)
	r.put(79, -500_000, 3_790_000)
	r.step("Discharging")
	// 模拟 daemon 死亡 2 小时期间又放了 400mAh，重启后 cc=4_591_667
	r.cur = r.cur.Add(2 * time.Minute)
	putCC(t, r, 4_591_667)
	r.put(70, -400_000, 3_700_000)
	r.rebuildPipeline() // restoreDischarge: lastCC=0 哨兵
	r.step("Discharging")
	putCC(t, r, 4_583_334) // 正常 -8333
	r.put(69, -400_000, 3_690_000)
	r.step("Discharging")
	r.put(65, 3_000_000, 3_600_000)
	r.step("Charging")
	rows, _ := r.st.RecentDischarge(5)
	if len(rows) != 1 {
		t.Fatalf("应落一行, got %d", len(rows))
	}
	// 只计重启后的 8333；死亡间隙 400000 与死亡前 8333 合计保留 8333(死亡前) + 8333(重启后)
	if want := int64(8333 + 8333); rows[0].Uah != want {
		t.Fatalf("跨间隙差分不应计入: uah=%d want=%d", rows[0].Uah, want)
	}
	if rows[0].StartCap != 80 {
		t.Fatalf("重启应延续原会话起点 80, got %d", rows[0].StartCap)
	}
}

// 掉幅不足：<5 不落行；5~9 落行但不给隐含。
func TestDischargeSmallDrops(t *testing.T) {
	r := newPipeRig(t)
	putCC(t, r, 5_000_000)
	r.put(80, -300_000, 3_800_000)
	r.step("Discharging")
	putCC(t, r, 4_900_000)
	r.put(77, -300_000, 3_770_000) // 掉幅 3
	r.step("Discharging")
	r.step("Discharging")
	r.put(77, 3_000_000, 3_700_000)
	r.step("Charging")
	if n := countRows(t, r.st, "discharge"); n != 0 {
		t.Fatalf("掉幅 3 不应落行, got %d", n)
	}

	// 掉幅 8：落行、无隐含
	putCC(t, r, 4_900_000)
	r.put(77, -300_000, 3_770_000)
	r.step("Charging") // 清掉旧会话（若有）
	putCC(t, r, 4_890_000)
	r.put(77, -300_000, 3_770_000)
	r.step("Discharging")
	putCC(t, r, 4_800_000)
	r.put(69, -300_000, 3_690_000) // 掉幅 8
	r.step("Discharging")
	r.put(69, 3_000_000, 3_600_000)
	r.step("Charging")
	rows, _ := r.st.RecentDischarge(5)
	if len(rows) != 1 || rows[0].Implied != nil {
		t.Fatalf("掉幅 8 应落行且无隐含, rows=%v", rows)
	}
	if rows[0].InvalidReason != "drop_lt_10" {
		t.Fatalf("invalid_reason = %q, want drop_lt_10", rows[0].InvalidReason)
	}
}

// charge_counter 节点缺失：静默跳过，不崩溃、不建会话。
func TestDischargeNodeAbsent(t *testing.T) {
	r := newPipeRig(t)
	r.put(80, -500_000, 3_800_000)
	r.step("Discharging")
	r.step("Discharging")
	if v, ok := r.st.KVGet(kvDisActive); ok && v == "1" {
		t.Fatal("节点缺失不应建放电会话")
	}
	if n := countRows(t, r.st, "discharge"); n != 0 {
		t.Fatalf("不应有放电行, got %d", n)
	}
}

// 放电样本降采样：60s 步长下每 5 分钟落一条，避免 samples 表膨胀
func TestDischargeSampleDownsampling(t *testing.T) {
	r := newPipeRig(t)
	cc := int64(5_000_000)
	putCC(t, r, cc)
	r.put(80, -500_000, 3_800_000)
	r.step("Discharging") // 首拍落一条（lastDisSampleTs 初值 0）
	// 后续 20 拍，每拍 60s：应仅在满 5 分钟（第 5 拍）再落一条
	for i := 0; i < 20; i++ {
		cc -= 8333
		putCC(t, r, cc)
		r.put(79, -500_000, 3_790_000)
		r.step("Discharging")
	}
	var n int64
	if err := r.st.db.QueryRow(`SELECT COUNT(*) FROM samples WHERE ua < 0`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	// 21 拍 × 60s = 21 分钟 ⇒ 首条 + 第 5/10/15/20 分钟共 5 条
	if n != 5 {
		t.Fatalf("放电样本应降采样为 5 条（21 分钟 / 5 分钟），got %d", n)
	}
}
