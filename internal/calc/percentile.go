package calc

import (
	"math"
	"sort"
)

// Percentile returns the p-th percentile (0 <= p <= 1) of values using the
// nearest-rank method. It does not mutate values. Used to turn Monte Carlo
// trial outcomes into p10/p50/p90 (SPEC.md 4.3).
func Percentile(values []int, p float64) int {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]int, len(values))
	copy(sorted, values)
	sort.Ints(sorted)

	idx := int(math.Round(p * float64(len(sorted)-1)))
	idx = min(max(idx, 0), len(sorted)-1)
	return sorted[idx]
}
