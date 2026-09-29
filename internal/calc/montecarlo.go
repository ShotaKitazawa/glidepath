package calc

import (
	"fmt"
	"math/rand/v2"
)

// YearlyPlan is one simulated year's cash-flow inputs (SPEC.md 4.3/4.4):
// income, planned expenses (trend regression + expense_forecasts +
// big_purchases for that year), and the planned NISA contribution.
type YearlyPlan struct {
	Year                int
	IncomeYen           int
	ExpenseYen          int
	NISAContributionYen int
}

// RunMonteCarlo runs the Monte Carlo simulation described in SPEC.md 4.3:
// for each of `trials` runs, walk plans in order, drawing 12 monthly
// returns with replacement from monthlyReturns and compounding them into an
// annual return applied to the NISA balance, while the bank balance moves
// by income - expense - NISA contribution. It returns, for each plan year,
// the net worth (bank + NISA) from every trial — callers reduce that to
// p10/p50/p90 via Percentile.
//
// plans must be supplied in the order they should be simulated (normally
// ascending by Year); RunMonteCarlo does not sort or deduplicate them.
func RunMonteCarlo(rng *rand.Rand, trials, initialBankYen, initialNISAYen int, monthlyReturns []float64, plans []YearlyPlan) (map[int][]int, error) {
	if trials <= 0 {
		return nil, fmt.Errorf("trials must be positive, got %d", trials)
	}
	if len(monthlyReturns) == 0 {
		return nil, fmt.Errorf("monthlyReturns must not be empty")
	}
	if len(plans) == 0 {
		return nil, fmt.Errorf("plans must not be empty")
	}

	results := make(map[int][]int, len(plans))
	for _, plan := range plans {
		results[plan.Year] = make([]int, trials)
	}

	for trial := range trials {
		bank := float64(initialBankYen)
		nisa := float64(initialNISAYen)
		for _, plan := range plans {
			r := sampleAnnualReturn(rng, monthlyReturns)
			nisa *= 1 + r
			nisa += float64(plan.NISAContributionYen)
			bank += float64(plan.IncomeYen - plan.ExpenseYen - plan.NISAContributionYen)
			results[plan.Year][trial] = int(bank + nisa)
		}
	}
	return results, nil
}

// sampleAnnualReturn draws 12 monthly returns with replacement from pool
// and compounds them into one annual return.
func sampleAnnualReturn(rng *rand.Rand, pool []float64) float64 {
	compound := 1.0
	for range 12 {
		compound *= 1 + pool[rng.IntN(len(pool))]
	}
	return compound - 1
}
