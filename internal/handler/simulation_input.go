package handler

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/ShotaKitazawa/glidepath/internal/calc"
	"github.com/ShotaKitazawa/glidepath/internal/database/sqlcgen"
)

// simulationEndYear is the end of the simulation horizon (SPEC.md section 2:
// 現在 〜 2047年3月、子の大学卒業).
const simulationEndYear = 2047

// simulationInput is everything RunMonteCarlo needs, assembled from the DB.
type simulationInput struct {
	InitialBankYen int
	InitialNISAYen int
	MonthlyReturns []float64
	Plans          []calc.YearlyPlan
	// ActualPoints is one net-worth figure per year that has at least one
	// monthly_records row (using that year's last-recorded month), computed
	// from real data only — no Monte Carlo. Plans covers the years after
	// the last ActualPoints year, through simulationEndYear.
	ActualPoints []actualPoint
}

// actualPoint is one year's actual (non-simulated) net worth.
type actualPoint struct {
	Year        int
	NetWorthYen int
}

// fundHistory is one NISA fund's NAV/contribution history, loaded once and
// reused both for the current valuation (nisaState) and for reconstructing
// past valuations at each ActualPoints year (nisaValuationAsOf).
type fundHistory struct {
	nav           []calc.NAVPoint
	contributions []calc.Contribution
}

// buildSimulationInput gathers the latest inventory, NISA holdings, family
// education-cost forecasts, and big purchases, and turns them into the
// inputs calc.RunMonteCarlo expects, following SPEC.md 4.3/4.4. Its error
// messages are shown directly to the user on the home page (見通し) when a
// prerequisite is missing, so they stay in Japanese and name what's needed.
func buildSimulationInput(ctx context.Context, q *sqlcgen.Queries) (*simulationInput, error) {
	records, err := q.ListMonthlyRecords(ctx)
	if err != nil {
		return nil, fmt.Errorf("棚卸し実績の読み込みに失敗しました: %w", err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("先に今月の記録を入力してください")
	}
	latest := records[0] // ListMonthlyRecords orders by record_month DESC
	currentYear := latest.RecordMonth.Time.Year()
	annualIncome := int(latest.IncomeMonthly) * 12

	history, err := historicalAnnualExpenses(ctx, q, records)
	if err != nil {
		return nil, err
	}

	fundHistories, err := loadFundHistories(ctx, q)
	if err != nil {
		return nil, err
	}

	monthsRecordedThisYear := 0
	for _, rec := range records {
		if rec.RecordMonth.Time.Year() == currentYear {
			monthsRecordedThisYear++
		}
	}
	initialNISAYen, monthlyReturns, annualNISAContribution := nisaState(fundHistories, currentYear, monthsRecordedThisYear, latest.RecordMonth.Time)

	actualPoints, err := actualNetWorthByYear(records, fundHistories)
	if err != nil {
		return nil, err
	}

	memberData, err := familyEducationData(ctx, q)
	if err != nil {
		return nil, err
	}

	bigPurchases, err := bigPurchaseData(ctx, q)
	if err != nil {
		return nil, err
	}

	totalContributedYen := 0
	for _, fh := range fundHistories {
		totalContributedYen += totalContributedAsOf(fh, latest.RecordMonth.Time)
	}

	// Plans cover only years after the last ActualPoints year: that year's
	// figure is already known exactly (it's the last entry of
	// ActualPoints), so RunMonteCarlo only needs to simulate forward from
	// there, not re-simulate a year that's already fact.
	plans := make([]calc.YearlyPlan, 0, simulationEndYear-currentYear)
	cumulativeContributedYen := totalContributedYen
	for year := currentYear + 1; year <= simulationEndYear; year++ {
		expense := calc.ProjectBaselineExpense(history, year)
		for _, md := range memberData {
			expense += calc.EducationCostForYear(md.birthYear, year, md.rows)
		}
		for _, bp := range bigPurchases {
			expense += calc.BigPurchaseCostForYear(bp, year)
		}

		var nisaContribution int
		nisaContribution, cumulativeContributedYen = capNISAContribution(annualNISAContribution, cumulativeContributedYen)

		plans = append(plans, calc.YearlyPlan{
			Year:                year,
			IncomeYen:           annualIncome,
			ExpenseYen:          expense,
			NISAContributionYen: nisaContribution,
		})
	}

	return &simulationInput{
		InitialBankYen: int(latest.BankBalance),
		InitialNISAYen: initialNISAYen,
		MonthlyReturns: monthlyReturns,
		Plans:          plans,
		ActualPoints:   actualPoints,
	}, nil
}

func historicalAnnualExpenses(ctx context.Context, q *sqlcgen.Queries, records []sqlcgen.MonthlyRecord) ([]calc.ExpensePoint, error) {
	type yearBucket struct {
		total  int
		months int
	}
	yearTotals := map[int]yearBucket{}
	for _, rec := range records {
		cats, err := q.ListExpenseCategoriesByMonthlyRecord(ctx, rec.ID)
		if err != nil {
			return nil, fmt.Errorf("支出履歴の読み込みに失敗しました: %w", err)
		}
		total := 0
		for _, c := range cats {
			total += int(c.Amount)
		}
		year := rec.RecordMonth.Time.Year()
		bucket := yearTotals[year]
		bucket.total += total
		bucket.months++
		yearTotals[year] = bucket
	}

	history := make([]calc.ExpensePoint, 0, len(yearTotals))
	for year, bucket := range yearTotals {
		history = append(history, calc.ExpensePoint{Year: year, TotalYen: calc.Annualize(bucket.total, bucket.months)})
	}
	sort.Slice(history, func(i, j int) bool { return history[i].Year < history[j].Year })
	return history, nil
}

// loadFundHistories reads every fund's NAV and contribution history once,
// skipping funds with no NAV history (nothing to compound returns from or
// price a valuation against).
func loadFundHistories(ctx context.Context, q *sqlcgen.Queries) ([]fundHistory, error) {
	funds, err := q.ListFunds(ctx)
	if err != nil {
		return nil, fmt.Errorf("ファンド情報の読み込みに失敗しました: %w", err)
	}

	histories := make([]fundHistory, 0, len(funds))
	for _, fund := range funds {
		// fundNavHistory follows funds.nav_proxy_fund_id when set, so a
		// fund with no data source of its own (e.g. no public API) still
		// gets priced using another fund's real movement (SPEC.md 4.1).
		navRows, err := fundNavHistory(ctx, q, fund)
		if err != nil {
			return nil, fmt.Errorf("基準価額履歴の読み込みに失敗しました: %w", err)
		}
		if len(navRows) == 0 {
			continue
		}
		nav := make([]calc.NAVPoint, len(navRows))
		for i, n := range navRows {
			nav[i] = calc.NAVPoint{Date: n.NavDate.Time, PriceYen: int(n.NavPrice)}
		}

		contribRows, err := q.ListNisaContributionsByFund(ctx, fund.ID)
		if err != nil {
			return nil, fmt.Errorf("拠出履歴の読み込みに失敗しました: %w", err)
		}
		contributions := make([]calc.Contribution, len(contribRows))
		for i, c := range contribRows {
			contributions[i] = calc.Contribution{
				Date:      c.ContributionDate.Time,
				AmountYen: int(c.Amount),
				Kind:      calc.ContributionKind(c.ContributionType),
			}
		}

		histories = append(histories, fundHistory{nav: nav, contributions: contributions})
	}
	return histories, nil
}

// nisaState derives the NISA valuation as of asOf, the monthly-return pool,
// and a full-year recurring contribution estimate. asOf must be the same
// date used for InitialBankYen (the latest monthly record) — otherwise the
// simulation's starting bank and NISA figures would be snapshots from two
// different points in time (real bug found 2026-09: this used to read
// time.Now(), while bank balance came from the latest monthly record,
// silently pulling in NISA activity the actual/実績 side hadn't caught up
// to yet). monthsRecordedThisYear annualizes the recurring estimate the
// same way historicalAnnualExpenses does for expenses (calc.Annualize) —
// currentYear is usually still in progress, so the raw sum-so-far would
// otherwise understate every future year.
func nisaState(fundHistories []fundHistory, currentYear, monthsRecordedThisYear int, asOf time.Time) (initialNISAYen int, monthlyReturns []float64, annualContribution int) {
	recurringThisYear := 0
	for _, fh := range fundHistories {
		monthlyReturns = append(monthlyReturns, calc.MonthlyReturns(fh.nav)...)
		for _, c := range fh.contributions {
			if c.Date.Year() == currentYear && c.Kind == calc.ContributionRecurring {
				recurringThisYear += c.AmountYen
			}
		}
		if valuation, ok := nisaValuationAsOf(fh, asOf); ok {
			initialNISAYen += valuation
		}
	}

	// Not requiring at least one fund's NAV history: RunMonteCarlo needs a
	// non-empty return series to sample from, but with no NISA holdings
	// (initialNISAYen and annualContribution both 0) a flat 0% series
	// leaves the simulation's NISA component at exactly 0 either way.
	if len(monthlyReturns) == 0 {
		monthlyReturns = []float64{0}
	}

	annualContribution = calc.Annualize(recurringThisYear, monthsRecordedThisYear)
	return initialNISAYen, monthlyReturns, annualContribution
}

// contributionsAsOf filters fh's contributions to those made on or before
// asOf, so callers can reconstruct a past state (valuation, cumulative
// principal) instead of using every contribution regardless of date.
func contributionsAsOf(fh fundHistory, asOf time.Time) []calc.Contribution {
	var contributions []calc.Contribution
	for _, c := range fh.contributions {
		if !c.Date.After(asOf) {
			contributions = append(contributions, c)
		}
	}
	return contributions
}

// totalContributedAsOf sums a fund's contribution principal (簿価) as of
// asOf — used to track progress against the NISA lifetime cap (SPEC.md 4.1).
func totalContributedAsOf(fh fundHistory, asOf time.Time) int {
	return calc.TotalContributedYen(contributionsAsOf(fh, asOf))
}

// capNISAContribution clamps a year's planned NISA contribution to Japan's
// annual and remaining-lifetime limits (principal/簿価 basis — SPEC.md 4.1),
// returning the capped amount and the resulting cumulative total. Whatever
// doesn't fit stays out of NISA; RunMonteCarlo's
// bank += income - expense - NISAContributionYen already sends it to bank
// instead, so no change is needed there.
func capNISAContribution(annualNISAContribution, cumulativeContributedYen int) (capped, newCumulative int) {
	capped = min(annualNISAContribution, calc.NISAAnnualLimitYen)
	if remaining := calc.NISALifetimeLimitYen - cumulativeContributedYen; capped > remaining {
		capped = max(remaining, 0)
	}
	return capped, cumulativeContributedYen + capped
}

// nisaValuationAsOf prices a fund's holdings using only the contributions
// made on or before asOf, so it can reconstruct a past valuation (for
// actualNetWorthByYear) the same way nisaState reads today's. ok is false
// only if there's no NAV price available on or before asOf at all.
func nisaValuationAsOf(fh fundHistory, asOf time.Time) (yen int, ok bool) {
	contributions := contributionsAsOf(fh, asOf)
	if len(contributions) == 0 {
		return 0, true
	}
	units, err := calc.HoldingUnits(contributions, fh.nav)
	if err != nil {
		return 0, false
	}
	valuation, err := calc.Valuation(units, fh.nav, asOf)
	if err != nil {
		return 0, false
	}
	return valuation, true
}

// actualNetWorthByYear returns one net-worth figure (bank balance + NISA
// valuation, no simulation) per year that has at least one monthly_records
// row, using that year's last-recorded month as the snapshot point.
func actualNetWorthByYear(records []sqlcgen.MonthlyRecord, fundHistories []fundHistory) ([]actualPoint, error) {
	// records is ordered by record_month DESC (ListMonthlyRecords), so the
	// first record seen for a given year is that year's latest month.
	latestByYear := map[int]sqlcgen.MonthlyRecord{}
	for _, rec := range records {
		year := rec.RecordMonth.Time.Year()
		if _, exists := latestByYear[year]; !exists {
			latestByYear[year] = rec
		}
	}

	points := make([]actualPoint, 0, len(latestByYear))
	for year, rec := range latestByYear {
		asOf := rec.RecordMonth.Time
		nisaYen := 0
		for _, fh := range fundHistories {
			valuation, ok := nisaValuationAsOf(fh, asOf)
			if !ok {
				return nil, fmt.Errorf("過去の評価額の計算に失敗しました")
			}
			nisaYen += valuation
		}
		points = append(points, actualPoint{Year: year, NetWorthYen: int(rec.BankBalance) + nisaYen})
	}
	sort.Slice(points, func(i, j int) bool { return points[i].Year < points[j].Year })
	return points, nil
}

type memberEducationData struct {
	birthYear int
	rows      []calc.ExpenseForecastRow
}

func familyEducationData(ctx context.Context, q *sqlcgen.Queries) ([]memberEducationData, error) {
	members, err := q.ListFamilyMembers(ctx)
	if err != nil {
		return nil, fmt.Errorf("家族情報の読み込みに失敗しました: %w", err)
	}

	data := make([]memberEducationData, 0, len(members))
	for _, m := range members {
		forecasts, err := q.ListExpenseForecastsByFamilyMember(ctx, m.ID)
		if err != nil {
			return nil, fmt.Errorf("教育費予測の読み込みに失敗しました: %w", err)
		}
		rows := make([]calc.ExpenseForecastRow, len(forecasts))
		for i, f := range forecasts {
			rows[i] = calc.ExpenseForecastRow{StartAge: int(f.StartAge), EndAge: int(f.EndAge), AnnualCostYen: int(f.AnnualCost)}
		}
		data = append(data, memberEducationData{birthYear: m.BirthMonth.Time.Year(), rows: rows})
	}
	return data, nil
}

func bigPurchaseData(ctx context.Context, q *sqlcgen.Queries) ([]calc.BigPurchase, error) {
	rows, err := q.ListBigPurchases(ctx)
	if err != nil {
		return nil, fmt.Errorf("大型出費の読み込みに失敗しました: %w", err)
	}

	purchases := make([]calc.BigPurchase, len(rows))
	for i, bp := range rows {
		rate := 0.0
		if bp.CategoryGrowthRate.Valid {
			if f, err := bp.CategoryGrowthRate.Float64Value(); err == nil && f.Valid {
				rate = f.Float64
			}
		}
		purchases[i] = calc.BigPurchase{
			BaseDate:        bp.BaseDate.Time,
			BaseAmountYen:   int(bp.BaseAmount),
			CycleYears:      int(bp.CycleYears),
			GrowthRate:      rate,
			TradeInValueYen: int(bp.TradeInValue),
			Recurring:       bp.Recurring,
		}
	}
	return purchases, nil
}
