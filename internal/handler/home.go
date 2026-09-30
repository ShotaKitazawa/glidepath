package handler

import (
	"context"
	"encoding/json"
	"html/template"
	"math/rand/v2"
	"net/http"

	"github.com/ShotaKitazawa/glidepath/internal/calc"
	"github.com/ShotaKitazawa/glidepath/internal/database/sqlcgen"
	"github.com/ShotaKitazawa/glidepath/internal/web"
)

// defaultTrials is the Monte Carlo trial count for the home page. It's
// fixed, not user-configurable, so there's nothing to set up before seeing
// an answer (SPEC.md 1: B — the forecast range — is the product).
const defaultTrials = 8000

// monteCarloSeed fixes the RNG so the same input data always produces the
// same forecast — see renderHome's rng.
const monteCarloSeed = 1

// registerHome wires GET / (見通し): the always-live Monte Carlo forecast.
// There is no "run" step — every visit recomputes from current data, so
// saving 今月の記録 (see inventory.go's redirect) is immediately reflected
// with no separate action.
func registerHome(mux *http.ServeMux, q *sqlcgen.Queries) {
	mux.HandleFunc("GET /{$}", homeIndex(q))
}

type simulationPointView struct {
	Year int32
	P10  int64
	P50  int64
	P90  int64
}

// actualPointView is one year's actual (non-simulated) net worth, in
// thousand yen — see simulation_input.go's actualPoint.
type actualPointView struct {
	Year        int32
	NetWorthYen int64
}

type homePageData struct {
	Title string
	// ActualPoints (実績) and ForecastPoints (予測) are rendered as two
	// visually distinct segments of the same chart/table — see the fan
	// chart's "実績" line vs its p10/p50/p90 bands.
	ActualPoints   []actualPointView
	ForecastPoints []simulationPointView
	ActualJSON     template.JS
	ForecastJSON   template.JS
	// Guidance replaces the chart when a prerequisite is missing (e.g. no
	// 今月の記録 yet) — buildSimulationInput's error message plus where to
	// go fix it.
	Guidance string
}

func homeIndex(q *sqlcgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		renderHome(w, r.Context(), q)
	}
}

func renderHome(w http.ResponseWriter, ctx context.Context, q *sqlcgen.Queries) {
	input, err := buildSimulationInput(ctx, q)
	if err != nil {
		web.Render(w, http.StatusOK, "home.html", homePageData{Title: "見通し", Guidance: err.Error()})
		return
	}

	// A fixed seed makes results reproducible across page loads for the
	// same input data — otherwise every visit reshuffles all 8,000 trials,
	// making it impossible to tell "the forecast changed" from "the RNG
	// landed differently this time" (requested 2026-09).
	rng := rand.New(rand.NewPCG(monteCarloSeed, monteCarloSeed))
	trialResults, err := calc.RunMonteCarlo(rng, defaultTrials, input.InitialBankYen, input.InitialNISAYen, input.MonthlyReturns, input.Plans)
	if err != nil {
		web.Render(w, http.StatusOK, "home.html", homePageData{Title: "見通し", Guidance: err.Error()})
		return
	}

	forecastPoints := make([]simulationPointView, 0, len(input.Plans))
	for _, yp := range input.Plans {
		forecastPoints = append(forecastPoints, simulationPointView{
			Year: int32(yp.Year),
			P10:  toThousandYen(calc.Percentile(trialResults[yp.Year], 0.1)),
			P50:  toThousandYen(calc.Percentile(trialResults[yp.Year], 0.5)),
			P90:  toThousandYen(calc.Percentile(trialResults[yp.Year], 0.9)),
		})
	}
	actualPoints := make([]actualPointView, 0, len(input.ActualPoints))
	for _, ap := range input.ActualPoints {
		actualPoints = append(actualPoints, actualPointView{
			Year:        int32(ap.Year),
			NetWorthYen: toThousandYen(ap.NetWorthYen),
		})
	}

	actualJSON, err := json.Marshal(actualPoints)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	forecastJSON, err := json.Marshal(forecastPoints)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	web.Render(w, http.StatusOK, "home.html", homePageData{
		Title:          "見通し",
		ActualPoints:   actualPoints,
		ForecastPoints: forecastPoints,
		ActualJSON:     template.JS(actualJSON),
		ForecastJSON:   template.JS(forecastJSON),
	})
}
