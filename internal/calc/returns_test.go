package calc

import "testing"

func closeEnough(a, b, eps float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= eps
}

func TestMonthlyReturns(t *testing.T) {
	// Two quotes in January (only the later one should count as the
	// month-end price), one in February, one in March.
	nav := []NAVPoint{
		{Date: date("2024-01-10"), PriceYen: 10_000},
		{Date: date("2024-01-31"), PriceYen: 11_000},
		{Date: date("2024-02-28"), PriceYen: 9_900},  // -10% vs Jan month-end
		{Date: date("2024-03-31"), PriceYen: 10_890}, // +10% vs Feb month-end
	}

	got := MonthlyReturns(nav)
	want := []float64{-0.1, 0.1}

	if len(got) != len(want) {
		t.Fatalf("got %d returns, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if !closeEnough(got[i], want[i], 1e-9) {
			t.Errorf("returns[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestMonthlyReturns_TooFewMonths(t *testing.T) {
	nav := []NAVPoint{{Date: date("2024-01-31"), PriceYen: 10_000}}
	if got := MonthlyReturns(nav); len(got) != 0 {
		t.Errorf("expected no returns from a single month, got %v", got)
	}
}
