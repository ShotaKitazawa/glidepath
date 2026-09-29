// Package calc implements the pure calculation logic behind SPEC.md
// section 4 (NISA valuation, net worth, Monte Carlo simulation, expense
// forecasting) and the education-cost defaults in section 5. Nothing here
// touches the database or network — callers assemble inputs from the DB
// layer and pass them in.
package calc

// educationDefault is one row of SPEC.md section 5's education cost table.
// annualCostYen is always an annual figure: for university the 4-year total
// from the table is divided by 4.
type educationDefault struct {
	stage         string
	track         string
	annualCostYen int
}

var educationDefaults = []educationDefault{
	{"幼稚園", "公立", 185_000},
	{"幼稚園", "私立", 347_000},
	{"小学校", "公立", 336_000},
	{"小学校", "私立", 1_828_000},
	{"中学校", "公立", 542_000},
	{"中学校", "私立", 1_560_000},
	{"高校", "公立", 590_000},
	{"高校", "私立", 1_020_000},
	{"大学", "国公立", 4_830_000 / 4},
	{"大学", "私立文系", 6_900_000 / 4},
	{"大学", "私立理系", 8_220_000 / 4},
}

// educationAgeRange holds the [startAge, endAge] (inclusive) a stage covers,
// independent of track.
var educationAgeRange = map[string][2]int{
	"幼稚園": {3, 5},
	"小学校": {6, 11},
	"中学校": {12, 14},
	"高校":  {15, 17},
	"大学":  {18, 21},
}

// DefaultAnnualEducationCost returns the default annual cost (in yen) for
// the given stage/track combination, per SPEC.md section 5. It returns
// ok=false for any stage/track not in the statistical table — callers must
// fall back to a manually entered value (expense_forecasts.is_override) in
// that case.
func DefaultAnnualEducationCost(stage, track string) (yen int, ok bool) {
	for _, d := range educationDefaults {
		if d.stage == stage && d.track == track {
			return d.annualCostYen, true
		}
	}
	return 0, false
}

// EducationAgeRange returns the inclusive [startAge, endAge] a stage
// spans, e.g. 小学校 -> (6, 11).
func EducationAgeRange(stage string) (startAge, endAge int, ok bool) {
	r, ok := educationAgeRange[stage]
	if !ok {
		return 0, 0, false
	}
	return r[0], r[1], true
}
