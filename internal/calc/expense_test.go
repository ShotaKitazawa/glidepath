package calc

import "testing"

func TestLinearRegression(t *testing.T) {
	// y = 2x + 1
	points := []Point{{X: 1, Y: 3}, {X: 2, Y: 5}, {X: 3, Y: 7}, {X: 4, Y: 9}}
	slope, intercept := LinearRegression(points)
	if !closeEnough(slope, 2, 1e-9) {
		t.Errorf("slope = %v, want 2", slope)
	}
	if !closeEnough(intercept, 1, 1e-9) {
		t.Errorf("intercept = %v, want 1", intercept)
	}
}

func TestLinearRegression_Degenerate(t *testing.T) {
	if slope, intercept := LinearRegression(nil); slope != 0 || intercept != 0 {
		t.Errorf("LinearRegression(nil) = (%v, %v), want (0, 0)", slope, intercept)
	}
	// A single point can't determine a slope; treat it as flat.
	if slope, intercept := LinearRegression([]Point{{X: 2024, Y: 4_000_000}}); slope != 0 || intercept != 4_000_000 {
		t.Errorf("LinearRegression(single point) = (%v, %v), want (0, 4000000)", slope, intercept)
	}
}

func TestProjectBaselineExpense(t *testing.T) {
	history := []ExpensePoint{
		{Year: 2022, TotalYen: 4_000_000},
		{Year: 2023, TotalYen: 4_200_000},
		{Year: 2024, TotalYen: 4_400_000},
	}
	got := ProjectBaselineExpense(history, 2026)
	want := 4_800_000 // continues the +200,000/year trend
	if got != want {
		t.Errorf("got %d, want %d", got, want)
	}
}

func TestProjectBaselineExpense_NoHistory(t *testing.T) {
	if got := ProjectBaselineExpense(nil, 2026); got != 0 {
		t.Errorf("got %d, want 0", got)
	}
}

func TestAnnualize(t *testing.T) {
	tests := []struct {
		name           string
		totalYen       int
		monthsRecorded int
		want           int
	}{
		{"full year unchanged", 3_600_000, 12, 3_600_000},
		{"two months scales to a full year", 600_000, 2, 3_600_000},
		{"one month scales to a full year", 300_000, 1, 3_600_000},
		{"zero months yields zero, not a division", 100_000, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Annualize(tt.totalYen, tt.monthsRecorded); got != tt.want {
				t.Errorf("Annualize(%d, %d) = %d, want %d", tt.totalYen, tt.monthsRecorded, got, tt.want)
			}
		})
	}
}

func TestEducationCostForYear(t *testing.T) {
	forecasts := []ExpenseForecastRow{
		{StartAge: 6, EndAge: 11, AnnualCostYen: 336_000},  // 小学校
		{StartAge: 12, EndAge: 14, AnnualCostYen: 542_000}, // 中学校
	}

	tests := []struct {
		birthYear  int
		targetYear int
		want       int
	}{
		{2015, 2022, 336_000}, // age 7, elementary
		{2015, 2027, 542_000}, // age 12, junior high
		{2015, 2016, 0},       // age 1, no forecast
	}
	for _, tt := range tests {
		if got := EducationCostForYear(tt.birthYear, tt.targetYear, forecasts); got != tt.want {
			t.Errorf("birthYear=%d targetYear=%d: got %d, want %d", tt.birthYear, tt.targetYear, got, tt.want)
		}
	}
}

func TestBigPurchaseCostForYear(t *testing.T) {
	bp := BigPurchase{
		BaseDate:        date("2024-06-01"),
		BaseAmountYen:   3_000_000,
		CycleYears:      6,
		GrowthRate:      0.1,
		TradeInValueYen: 200_000,
		Recurring:       true,
	}

	tests := []struct {
		year int
		want int
	}{
		{2023, 0},                                // before base year
		{2024, 3_000_000 - 200_000},              // base occurrence
		{2027, 0},                                // off-cycle
		{2030, int(3_000_000*1.1) - 200_000},     // one cycle later, grown 10%
		{2036, int(3_000_000*1.1*1.1) - 200_000}, // two cycles later
	}
	for _, tt := range tests {
		if got := BigPurchaseCostForYear(bp, tt.year); got != tt.want {
			t.Errorf("year=%d: got %d, want %d", tt.year, got, tt.want)
		}
	}
}

func TestBigPurchaseCostForYear_NonRecurringOnlyOnce(t *testing.T) {
	bp := BigPurchase{
		BaseDate:      date("2024-06-01"),
		BaseAmountYen: 1_000_000,
		CycleYears:    6,
		Recurring:     false,
	}
	if got := BigPurchaseCostForYear(bp, 2024); got != 1_000_000 {
		t.Errorf("base year: got %d, want 1000000", got)
	}
	if got := BigPurchaseCostForYear(bp, 2030); got != 0 {
		t.Errorf("next cycle for non-recurring purchase: got %d, want 0", got)
	}
}

func TestBigPurchaseCostForYear_OneOff(t *testing.T) {
	bp := BigPurchase{
		BaseDate:      date("2025-04-01"),
		BaseAmountYen: 500_000,
		CycleYears:    0,
		Recurring:     false,
	}
	if got := BigPurchaseCostForYear(bp, 2025); got != 500_000 {
		t.Errorf("got %d, want 500000", got)
	}
	if got := BigPurchaseCostForYear(bp, 2026); got != 0 {
		t.Errorf("got %d, want 0", got)
	}
}
