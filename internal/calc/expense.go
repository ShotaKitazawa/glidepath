package calc

import (
	"math"
	"time"
)

// Point is a generic (x, y) sample for LinearRegression.
type Point struct {
	X, Y float64
}

// LinearRegression fits y = slope*x + intercept via ordinary least squares.
// Fewer than two points (or all-identical x) yields slope=0 and intercept
// equal to the single/last y value, i.e. a flat projection.
func LinearRegression(points []Point) (slope, intercept float64) {
	n := float64(len(points))
	if n == 0 {
		return 0, 0
	}
	if n == 1 {
		return 0, points[0].Y
	}

	var sumX, sumY, sumXY, sumXX float64
	for _, p := range points {
		sumX += p.X
		sumY += p.Y
		sumXY += p.X * p.Y
		sumXX += p.X * p.X
	}

	denom := n*sumXX - sumX*sumX
	if denom == 0 {
		return 0, sumY / n
	}
	slope = (n*sumXY - sumX*sumY) / denom
	intercept = (sumY - slope*sumX) / n
	return slope, intercept
}

// ExpensePoint is one year's actual total expense, aggregated from
// monthly_records/expense_categories (SPEC.md 4.4: 実績トレンド回帰).
// TotalYen must already be a full-year figure — see Annualize for a
// year with fewer than 12 months recorded.
type ExpensePoint struct {
	Year     int
	TotalYen int
}

// Annualize scales a sum recorded across only part of a year up to a
// full-year equivalent. Used for both historical expense totals and NISA
// recurring-contribution totals (SPEC.md 4.3/4.4) — passing a raw
// partial-year sum unscaled would understate the full-year figure (and,
// for expenses, overstate projected assets) in direct proportion to how
// much of the year is still unrecorded.
func Annualize(totalYen, monthsRecorded int) int {
	if monthsRecorded <= 0 {
		return 0
	}
	return totalYen * 12 / monthsRecorded
}

// ProjectBaselineExpense linearly extrapolates historical annual expense
// totals to targetYear. Returns 0 with no history.
func ProjectBaselineExpense(history []ExpensePoint, targetYear int) int {
	if len(history) == 0 {
		return 0
	}
	points := make([]Point, len(history))
	for i, h := range history {
		points[i] = Point{X: float64(h.Year), Y: float64(h.TotalYen)}
	}
	slope, intercept := LinearRegression(points)
	return int(slope*float64(targetYear) + intercept)
}

// ExpenseForecastRow is one expense_forecasts entry (SPEC.md section 3):
// an annual cost that applies while the family member's age is within
// [StartAge, EndAge].
type ExpenseForecastRow struct {
	StartAge      int
	EndAge        int
	AnnualCostYen int
}

// EducationCostForYear sums the expense_forecasts rows that apply to a
// family member born in birthYear during targetYear, based on age.
func EducationCostForYear(birthYear, targetYear int, forecasts []ExpenseForecastRow) int {
	age := targetYear - birthYear
	total := 0
	for _, f := range forecasts {
		if age >= f.StartAge && age <= f.EndAge {
			total += f.AnnualCostYen
		}
	}
	return total
}

// BigPurchase mirrors a big_purchases row (SPEC.md section 3).
type BigPurchase struct {
	BaseDate        time.Time
	BaseAmountYen   int
	CycleYears      int
	GrowthRate      float64 // category_growth_rate; 0 if unset
	TradeInValueYen int
	Recurring       bool
}

// BigPurchaseCostForYear returns the net cost (base amount grown by
// GrowthRate per elapsed cycle, minus trade-in value) that a big purchase
// contributes to targetYear, or 0 if it doesn't recur in that year.
// CycleYears <= 0 is treated as a one-off purchase in the base year only.
func BigPurchaseCostForYear(bp BigPurchase, targetYear int) int {
	baseYear := bp.BaseDate.Year()
	diff := targetYear - baseYear
	if diff < 0 {
		return 0
	}

	if bp.CycleYears <= 0 {
		if diff == 0 {
			return bp.BaseAmountYen - bp.TradeInValueYen
		}
		return 0
	}

	if diff%bp.CycleYears != 0 {
		return 0
	}
	cycles := diff / bp.CycleYears
	if cycles > 0 && !bp.Recurring {
		return 0
	}

	amount := float64(bp.BaseAmountYen) * math.Pow(1+bp.GrowthRate, float64(cycles))
	return int(amount) - bp.TradeInValueYen
}
