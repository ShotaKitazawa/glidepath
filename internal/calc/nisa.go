package calc

import (
	"fmt"
	"sort"
	"time"
)

// NAVPoint is one day's 基準価額 (NAV price per 10,000 units, in yen), as
// cached in fund_nav_history.
type NAVPoint struct {
	Date     time.Time
	PriceYen int
}

// ContributionKind classifies a nisa_contributions row along two independent
// axes (SPEC.md 4.1/4.3): whether it should feed a future recurring-pace
// estimate, and whether it represents money added on top of the running
// total (Recurring/Spot) or a declared total that supersedes everything
// before it (Snapshot) — see ActiveContributions.
type ContributionKind string

const (
	// ContributionRecurring is a steady, expected monthly deposit: counts
	// toward both units/yen totals and the recurring-pace estimate.
	ContributionRecurring ContributionKind = "recurring"
	// ContributionSpot is a genuine one-off deposit (money that really was
	// added that day) that shouldn't be extrapolated as a future pace:
	// counts toward units/yen totals, excluded from the pace estimate.
	ContributionSpot ContributionKind = "spot"
	// ContributionSnapshot declares "the fund was worth AmountYen as of
	// Date" (used by the initial-holding backfill, not a deposit at all).
	// It is not a positive addition to the running total — it replaces
	// everything before it, since that value is already reflected in the
	// declared amount. See ActiveContributions.
	ContributionSnapshot ContributionKind = "snapshot"
)

// Contribution is one NISA deposit or valuation snapshot (nisa_contributions
// row).
type Contribution struct {
	Date      time.Time
	AmountYen int
	Kind      ContributionKind
}

// ActiveContributions returns the contributions that should count toward
// units/yen totals. If the most recent ContributionSnapshot among them
// exists, everything on or before its date — including other contributions
// that happen to share that date — is superseded (its declared amount
// already reflects their cumulative effect as of that date); only the
// snapshot itself and anything strictly after it remain. With no snapshot,
// every contribution is active. contributions need not be sorted; the
// returned slice is sorted ascending by Date.
func ActiveContributions(contributions []Contribution) []Contribution {
	sorted := make([]Contribution, len(contributions))
	copy(sorted, contributions)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Date.Before(sorted[j].Date) })

	lastSnapshot := -1
	for i, c := range sorted {
		if c.Kind == ContributionSnapshot {
			lastSnapshot = i
		}
	}
	if lastSnapshot == -1 {
		return sorted
	}
	return sorted[lastSnapshot:]
}

// TotalContributedYen sums ActiveContributions' declared amounts: the
// fund's cumulative contribution principal (簿価), as opposed to its
// current market valuation (Valuation). Used both for display (累計拠出額)
// and for tracking progress against Japan's NISA lifetime cap — see
// NISAAnnualLimitYen/NISALifetimeLimitYen (SPEC.md 4.1).
func TotalContributedYen(contributions []Contribution) int {
	total := 0
	for _, c := range ActiveContributions(contributions) {
		total += c.AmountYen
	}
	return total
}

// NISAAnnualLimitYen and NISALifetimeLimitYen are Japan's 新NISA (2024〜)
// contribution caps, in principal/簿価 terms: 年360万円(つみたて投資枠120万+
// 成長投資枠240万、合算) と生涯1,800万円。The rule that a later sale frees up
// lifetime room again (簿価残高方式) is intentionally not modeled — this app
// never simulates selling, only accumulating, so that rule would add
// complexity with no scenario that could ever exercise it (SPEC.md 4.1).
const (
	NISAAnnualLimitYen   = 3_600_000
	NISALifetimeLimitYen = 18_000_000
)

const unitsPerNAVQuote = 10_000

// navOnOrBefore returns the NAV price in effect on d: the latest quote with
// Date <= d. nav must be sorted ascending by Date.
func navOnOrBefore(nav []NAVPoint, d time.Time) (int, bool) {
	price := 0
	found := false
	for _, p := range nav {
		if p.Date.After(d) {
			break
		}
		price = p.PriceYen
		found = true
	}
	return price, found
}

// HoldingUnits computes total held units (口数) from a contribution
// history and the fund's NAV history (SPEC.md 4.1: 保有口数の増分 = 入金額 /
// 入金日の基準価額). Only ActiveContributions(contributions) are counted, so
// a later ContributionSnapshot supersedes earlier deposits instead of
// double-counting them. nav need not be sorted; contributions need not be
// sorted.
func HoldingUnits(contributions []Contribution, nav []NAVPoint) (float64, error) {
	sorted := make([]NAVPoint, len(nav))
	copy(sorted, nav)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Date.Before(sorted[j].Date) })

	var units float64
	for _, c := range ActiveContributions(contributions) {
		price, ok := navOnOrBefore(sorted, c.Date)
		if !ok {
			return 0, fmt.Errorf("no NAV price available on or before contribution date %s", c.Date.Format("2006-01-02"))
		}
		if price <= 0 {
			return 0, fmt.Errorf("invalid NAV price %d on %s", price, c.Date.Format("2006-01-02"))
		}
		units += float64(c.AmountYen) * unitsPerNAVQuote / float64(price)
	}
	return units, nil
}

// Valuation computes the yen valuation of the given unit holding as of
// asOf, using the latest NAV quote on or before asOf (SPEC.md 4.1: NISA評価額
// = Σ(保有口数) × 現在の基準価額).
func Valuation(units float64, nav []NAVPoint, asOf time.Time) (int, error) {
	sorted := make([]NAVPoint, len(nav))
	copy(sorted, nav)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Date.Before(sorted[j].Date) })

	price, ok := navOnOrBefore(sorted, asOf)
	if !ok {
		return 0, fmt.Errorf("no NAV price available on or before %s", asOf.Format("2006-01-02"))
	}
	return int(units * float64(price) / unitsPerNAVQuote), nil
}
