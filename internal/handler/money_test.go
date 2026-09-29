package handler

import "testing"

func TestToThousandYen(t *testing.T) {
	cases := []struct {
		name string
		yen  int
		want int32
	}{
		{"zero", 0, 0},
		{"typical amount", 1_915_000, 1915},
		{"truncates remainder", 1500, 1},
		{"negative", -2000, -2},
		{
			"exceeds int32 range in raw yen but not after dividing by 1000",
			5_004_550_000, // a 20-year Monte Carlo p90 forecast can plausibly exceed int32's ~2.1B yen range
			5_004_550,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := toThousandYen(c.yen)
			if got != c.want {
				t.Errorf("toThousandYen(%d) = %d, want %d", c.yen, got, c.want)
			}
		})
	}
}
