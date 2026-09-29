package handler

import (
	"testing"
	"time"

	"github.com/ShotaKitazawa/glidepath/internal/calc"
)

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestCapNISAContribution(t *testing.T) {
	cases := []struct {
		name                     string
		annualNISAContribution   int
		cumulativeContributedYen int
		wantCapped               int
		wantNewCumulative        int
	}{
		{"under both limits: unchanged", 600_000, 1_000_000, 600_000, 1_600_000},
		{"over the annual limit: capped to it", 5_000_000, 0, calc.NISAAnnualLimitYen, calc.NISAAnnualLimitYen},
		{"exactly at the lifetime limit already: capped to zero", 600_000, calc.NISALifetimeLimitYen, 0, calc.NISALifetimeLimitYen},
		{"would cross the lifetime limit: capped to remaining room", 1_000_000, calc.NISALifetimeLimitYen - 300_000, 300_000, calc.NISALifetimeLimitYen},
		{"already past the lifetime limit: capped to zero, not negative", 600_000, calc.NISALifetimeLimitYen + 500_000, 0, calc.NISALifetimeLimitYen + 500_000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotCapped, gotNewCumulative := capNISAContribution(c.annualNISAContribution, c.cumulativeContributedYen)
			if gotCapped != c.wantCapped || gotNewCumulative != c.wantNewCumulative {
				t.Errorf("capNISAContribution(%d, %d) = (%d, %d), want (%d, %d)",
					c.annualNISAContribution, c.cumulativeContributedYen,
					gotCapped, gotNewCumulative, c.wantCapped, c.wantNewCumulative)
			}
		})
	}
}

func TestNisaState_ExcludesSpotContributionsFromAnnualEstimate(t *testing.T) {
	// A steady 50,000/month recurring pace for 8 of the year's 12 months,
	// plus a one-off backdated initial-holding snapshot that must not be
	// replayed into every future forecast year (see SPEC.md 4.3).
	fh := fundHistory{
		contributions: []calc.Contribution{
			{Date: date(2026, time.January, 1), AmountYen: 50_000, Kind: calc.ContributionRecurring},
			{Date: date(2026, time.February, 1), AmountYen: 50_000, Kind: calc.ContributionRecurring},
			{Date: date(2026, time.March, 1), AmountYen: 50_000, Kind: calc.ContributionRecurring},
			{Date: date(2026, time.April, 1), AmountYen: 50_000, Kind: calc.ContributionRecurring},
			{Date: date(2026, time.May, 1), AmountYen: 50_000, Kind: calc.ContributionRecurring},
			{Date: date(2026, time.June, 1), AmountYen: 50_000, Kind: calc.ContributionRecurring},
			{Date: date(2026, time.July, 1), AmountYen: 50_000, Kind: calc.ContributionRecurring},
			{Date: date(2026, time.August, 1), AmountYen: 50_000, Kind: calc.ContributionRecurring},
			{Date: date(2026, time.September, 18), AmountYen: 1_915_000, Kind: calc.ContributionSnapshot},
		},
	}

	_, _, annualContribution := nisaState([]fundHistory{fh}, 2026, 8, date(2026, time.December, 31))

	// Recurring-only sum is 400,000 over 8 recorded months; annualized to
	// 12 months that's 600,000. The 1,915,000 snapshot backfill must not
	// appear here at all, let alone be replayed across 21 forecast years.
	want := 600_000
	if annualContribution != want {
		t.Errorf("annualContribution = %d, want %d (snapshot contribution leaked into the recurring estimate)", annualContribution, want)
	}
}

func TestNisaState_ValuationRespectsAsOfNotWallClock(t *testing.T) {
	// A contribution dated after asOf (the latest monthly record's month)
	// must not count toward initialNISAYen, even though it's already in
	// the past relative to the real wall clock — asOf must match whatever
	// date InitialBankYen came from, or the simulation's starting bank and
	// NISA figures are snapshots from two different points in time (real
	// bug found 2026-09).
	fh := fundHistory{
		nav: []calc.NAVPoint{
			{Date: date(2026, time.August, 1), PriceYen: 10_000},
			{Date: date(2026, time.September, 18), PriceYen: 10_000},
		},
		contributions: []calc.Contribution{
			{Date: date(2026, time.August, 1), AmountYen: 100_000, Kind: calc.ContributionRecurring},
			{Date: date(2026, time.September, 18), AmountYen: 1_915_000, Kind: calc.ContributionSnapshot},
		},
	}

	asOf := date(2026, time.August, 1) // the latest monthly record is for August
	initialNISAYen, _, _ := nisaState([]fundHistory{fh}, 2026, 1, asOf)

	// Only the August contribution is on or before asOf; the September
	// snapshot must be excluded entirely, not just from the trend estimate.
	const want = 100_000
	if initialNISAYen != want {
		t.Errorf("initialNISAYen = %d, want %d (a contribution after asOf leaked into the valuation)", initialNISAYen, want)
	}
}

func TestNisaState_NoRecurringContributionsYieldsZero(t *testing.T) {
	fh := fundHistory{
		contributions: []calc.Contribution{
			{Date: date(2026, time.September, 18), AmountYen: 1_915_000, Kind: calc.ContributionSnapshot},
		},
	}

	_, _, annualContribution := nisaState([]fundHistory{fh}, 2026, 8, date(2026, time.December, 31))

	if annualContribution != 0 {
		t.Errorf("annualContribution = %d, want 0 (only a snapshot contribution exists this year)", annualContribution)
	}
}
