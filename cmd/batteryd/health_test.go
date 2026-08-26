package main

import "testing"

func TestHealthPct(t *testing.T) {
	cases := []struct {
		full, design, want int64
	}{
		{6039000, 6300000, 95},
		{6300000, 6300000, 100},
		{3150000, 6300000, 50},
		{0, 6300000, 0},
		{6300000, 0, 0},      // 非法设计容量，守卫
		{6300000, -1, 0},     // 负值守卫
	}
	for _, c := range cases {
		if got := healthPct(c.full, c.design); got != c.want {
			t.Errorf("healthPct(%d,%d)=%d want %d", c.full, c.design, got, c.want)
		}
	}
}
