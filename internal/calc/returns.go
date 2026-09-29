package calc

import "sort"

// MonthlyReturns turns a (possibly daily) NAV history into a month-over-month
// return series, using each month's latest quote as its month-end price
// (SPEC.md 4.3: モンテカルロは設定来の月次リターンをプールとして使う). Returns nil if
// fewer than two distinct months are present.
func MonthlyReturns(nav []NAVPoint) []float64 {
	type monthKey struct {
		year  int
		month int
	}

	latestInMonth := map[monthKey]NAVPoint{}
	for _, p := range nav {
		k := monthKey{p.Date.Year(), int(p.Date.Month())}
		if existing, ok := latestInMonth[k]; !ok || p.Date.After(existing.Date) {
			latestInMonth[k] = p
		}
	}

	monthEnds := make([]NAVPoint, 0, len(latestInMonth))
	for _, p := range latestInMonth {
		monthEnds = append(monthEnds, p)
	}
	sort.Slice(monthEnds, func(i, j int) bool { return monthEnds[i].Date.Before(monthEnds[j].Date) })

	if len(monthEnds) < 2 {
		return nil
	}

	returns := make([]float64, 0, len(monthEnds)-1)
	for i := 1; i < len(monthEnds); i++ {
		prev := float64(monthEnds[i-1].PriceYen)
		cur := float64(monthEnds[i].PriceYen)
		returns = append(returns, cur/prev-1)
	}
	return returns
}
