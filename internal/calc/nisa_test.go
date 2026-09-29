package calc

import (
	"testing"
	"time"
)

func date(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestNISAHoldingUnits(t *testing.T) {
	nav := []NAVPoint{
		{Date: date("2024-01-01"), PriceYen: 10_000}, // 1万口 = 1万円 → 1円/口
		{Date: date("2024-02-01"), PriceYen: 20_000}, // 1万口 = 2万円 → 2円/口
	}
	contributions := []Contribution{
		{Date: date("2024-01-01"), AmountYen: 10_000}, // buys 10,000 units at 1円/口
		{Date: date("2024-02-01"), AmountYen: 20_000}, // buys 10,000 units at 2円/口
	}

	units, err := HoldingUnits(contributions, nav)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	const want = 20_000.0
	if diff := units - want; diff > 1e-6 || diff < -1e-6 {
		t.Errorf("units = %v, want %v", units, want)
	}
}

func TestNISAHoldingUnits_NoNAVBeforeContribution(t *testing.T) {
	nav := []NAVPoint{
		{Date: date("2024-02-01"), PriceYen: 10_000},
	}
	contributions := []Contribution{
		{Date: date("2024-01-01"), AmountYen: 10_000},
	}

	if _, err := HoldingUnits(contributions, nav); err == nil {
		t.Fatal("expected error when no NAV price is available on or before the contribution date")
	}
}

func TestNISAValuation(t *testing.T) {
	nav := []NAVPoint{
		{Date: date("2024-01-01"), PriceYen: 10_000},
		{Date: date("2024-06-01"), PriceYen: 15_000},
	}
	units := 20_000.0 // 2万口

	got, err := Valuation(units, nav, date("2024-06-15"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 20,000口 * (15,000円 / 10,000口) = 30,000円
	const want = 30_000
	if got != want {
		t.Errorf("got %d, want %d", got, want)
	}
}

func TestValuation_NoNAVAsOfDate(t *testing.T) {
	nav := []NAVPoint{
		{Date: date("2024-06-01"), PriceYen: 10_000},
	}
	if _, err := Valuation(1000, nav, date("2024-01-01")); err == nil {
		t.Fatal("expected error when no NAV price is available on or before asOf")
	}
}

func TestHoldingUnits_SnapshotSupersedesEarlierContributions(t *testing.T) {
	// A real deposit, then a later initial-holding snapshot declaring the
	// fund's total value as of that date. The snapshot's declared value
	// already reflects the earlier deposit's cumulative effect, so it must
	// replace it rather than stack on top (the deposit would otherwise be
	// double-counted — real bug found 2026-09).
	nav := []NAVPoint{
		{Date: date("2024-01-01"), PriceYen: 10_000},
		{Date: date("2024-06-01"), PriceYen: 20_000},
	}
	contributions := []Contribution{
		{Date: date("2024-01-01"), AmountYen: 10_000, Kind: ContributionRecurring}, // buys 10,000 units at 1円/口
		{Date: date("2024-06-01"), AmountYen: 30_000, Kind: ContributionSnapshot},  // declares 30,000円 total as of this date
	}

	units, err := HoldingUnits(contributions, nav)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only the snapshot counts: 30,000円 / 2円 per unit = 15,000 units. The
	// January deposit's 10,000 units must NOT also be added on top.
	const want = 15_000.0
	if diff := units - want; diff > 1e-6 || diff < -1e-6 {
		t.Errorf("units = %v, want %v (January deposit was double-counted alongside the snapshot)", units, want)
	}
}

func TestHoldingUnits_ContributionsAfterSnapshotStillAdd(t *testing.T) {
	nav := []NAVPoint{
		{Date: date("2024-01-01"), PriceYen: 10_000},
		{Date: date("2024-06-01"), PriceYen: 20_000},
	}
	contributions := []Contribution{
		{Date: date("2024-01-01"), AmountYen: 30_000, Kind: ContributionSnapshot},  // 30,000 units as of Jan (1円/口)
		{Date: date("2024-06-01"), AmountYen: 20_000, Kind: ContributionRecurring}, // +10,000 units in June (2円/口)
	}

	units, err := HoldingUnits(contributions, nav)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	const want = 40_000.0
	if diff := units - want; diff > 1e-6 || diff < -1e-6 {
		t.Errorf("units = %v, want %v", units, want)
	}
}

func TestActiveContributions_NoSnapshotKeepsEverything(t *testing.T) {
	contributions := []Contribution{
		{Date: date("2024-02-01"), AmountYen: 100, Kind: ContributionRecurring},
		{Date: date("2024-01-01"), AmountYen: 200, Kind: ContributionSpot},
	}
	active := ActiveContributions(contributions)
	if len(active) != 2 {
		t.Fatalf("len(active) = %d, want 2", len(active))
	}
	if !active[0].Date.Equal(date("2024-01-01")) || !active[1].Date.Equal(date("2024-02-01")) {
		t.Errorf("expected ascending date order, got %v then %v", active[0].Date, active[1].Date)
	}
}

func TestActiveContributions_UsesLatestSnapshotOnly(t *testing.T) {
	contributions := []Contribution{
		{Date: date("2024-01-01"), AmountYen: 100, Kind: ContributionRecurring},
		{Date: date("2024-02-01"), AmountYen: 200, Kind: ContributionSnapshot},
		{Date: date("2024-03-01"), AmountYen: 300, Kind: ContributionRecurring},
		{Date: date("2024-04-01"), AmountYen: 400, Kind: ContributionSnapshot},
		{Date: date("2024-05-01"), AmountYen: 500, Kind: ContributionRecurring},
	}
	active := ActiveContributions(contributions)
	if len(active) != 2 {
		t.Fatalf("len(active) = %d, want 2 (the 04-01 snapshot and the 05-01 deposit)", len(active))
	}
	if !active[0].Date.Equal(date("2024-04-01")) || !active[1].Date.Equal(date("2024-05-01")) {
		t.Errorf("expected 04-01 then 05-01, got %v then %v", active[0].Date, active[1].Date)
	}
}
