package calc

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestRunMonteCarlo_DeterministicWithSingleReturn(t *testing.T) {
	// With only one possible monthly return, every trial is identical and
	// exactly computable by hand.
	monthlyReturns := []float64{0.1}
	plans := []YearlyPlan{
		{Year: 2025, IncomeYen: 6_000_000, ExpenseYen: 4_000_000, NISAContributionYen: 1_200_000},
		{Year: 2026, IncomeYen: 6_000_000, ExpenseYen: 4_500_000, NISAContributionYen: 1_200_000},
	}

	rng := rand.New(rand.NewPCG(1, 2))
	results, err := RunMonteCarlo(rng, 3, 1_000_000, 500_000, monthlyReturns, plans)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	annualReturn := math.Pow(1.1, 12) - 1

	bank := 1_000_000.0
	nisa := 500_000.0
	wantByYear := map[int]int{}
	for _, p := range plans {
		nisa *= 1 + annualReturn
		nisa += float64(p.NISAContributionYen)
		bank += float64(p.IncomeYen - p.ExpenseYen - p.NISAContributionYen)
		wantByYear[p.Year] = int(bank + nisa)
	}

	for year, want := range wantByYear {
		trials, ok := results[year]
		if !ok {
			t.Fatalf("no results for year %d", year)
		}
		if len(trials) != 3 {
			t.Fatalf("expected 3 trials for year %d, got %d", year, len(trials))
		}
		for i, got := range trials {
			if got != want {
				t.Errorf("year %d trial %d = %d, want %d", year, i, got, want)
			}
		}
	}
}

func TestRunMonteCarlo_PercentilesAreOrdered(t *testing.T) {
	monthlyReturns := []float64{-0.15, -0.05, 0.02, 0.1, 0.2}
	plans := []YearlyPlan{
		{Year: 2025, IncomeYen: 6_000_000, ExpenseYen: 4_000_000, NISAContributionYen: 1_200_000},
		{Year: 2030, IncomeYen: 6_000_000, ExpenseYen: 4_000_000, NISAContributionYen: 1_200_000},
	}

	rng := rand.New(rand.NewPCG(7, 42))
	results, err := RunMonteCarlo(rng, 2000, 1_000_000, 3_000_000, monthlyReturns, plans)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	trials := results[2030]
	p10 := Percentile(trials, 0.1)
	p50 := Percentile(trials, 0.5)
	p90 := Percentile(trials, 0.9)

	if !(p10 <= p50 && p50 <= p90) {
		t.Errorf("expected p10 <= p50 <= p90, got p10=%d p50=%d p90=%d", p10, p50, p90)
	}
	if p10 == p90 {
		t.Errorf("expected spread between p10 and p90 given return variance, both = %d", p10)
	}
}

func TestRunMonteCarlo_Validation(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 1))
	plans := []YearlyPlan{{Year: 2025}}

	if _, err := RunMonteCarlo(rng, 0, 0, 0, []float64{0.1}, plans); err == nil {
		t.Error("expected error for trials <= 0")
	}
	if _, err := RunMonteCarlo(rng, 10, 0, 0, nil, plans); err == nil {
		t.Error("expected error for empty monthlyReturns")
	}
	if _, err := RunMonteCarlo(rng, 10, 0, 0, []float64{0.1}, nil); err == nil {
		t.Error("expected error for empty plans")
	}
}
