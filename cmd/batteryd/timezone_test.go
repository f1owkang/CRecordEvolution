package main

import (
	"testing"
	"time"
)

func withTzSources(t *testing.T, prop func() (string, bool), file func() (string, bool)) {
	t.Helper()
	oldProp, oldFile := tzPropSource, tzFileSource
	tzPropSource, tzFileSource = prop, file
	t.Cleanup(func() { tzPropSource, tzFileSource = oldProp, oldFile })
}

func TestResolveLocalZoneFromGetprop(t *testing.T) {
	withTzSources(t,
		func() (string, bool) { return "Asia/Shanghai", true },
		func() (string, bool) { t.Fatal("getprop 命中后不应再读文件"); return "", false })
	loc := resolveLocalZone()
	// 内嵌 tzdata 保证 LoadLocation 不依赖系统文件；上海 1 月偏移 +8h
	if _, off := time.Date(2026, 1, 1, 12, 0, 0, 0, loc).Zone(); off != 8*3600 {
		t.Fatalf("Asia/Shanghai 冬季应 UTC+8, got offset %d", off)
	}
}

func TestResolveLocalZoneFallsBackToPropertyFile(t *testing.T) {
	withTzSources(t,
		func() (string, bool) { return "", false },
		func() (string, bool) { return "Europe/Berlin", true })
	loc := resolveLocalZone()
	if _, off := time.Date(2026, 7, 1, 12, 0, 0, 0, loc).Zone(); off != 2*3600 {
		t.Fatalf("Europe/Berlin 夏季应 UTC+2, got offset %d", off)
	}
}

func TestResolveLocalZoneInvalidNameFallsThrough(t *testing.T) {
	// getprop 返回非 IANA 名（如时区缩写 CST）应跳过并落到下一来源
	withTzSources(t,
		func() (string, bool) { return "CST", true },
		func() (string, bool) { return "Asia/Tokyo", true })
	loc := resolveLocalZone()
	if _, off := time.Date(2026, 1, 1, 12, 0, 0, 0, loc).Zone(); off != 9*3600 {
		t.Fatalf("非法名应顺延到文件源取 Asia/Tokyo(+9), got offset %d", off)
	}
}

func TestResolveLocalZoneHonestUTC(t *testing.T) {
	withTzSources(t,
		func() (string, bool) { return "", false },
		func() (string, bool) { return "", false })
	if got := resolveLocalZone(); got != time.Local {
		t.Fatalf("全部来源失败应保持 time.Local（诚实 UTC），不硬编码时区, got %v", got)
	}
}
