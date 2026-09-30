// Package handler registers the application's HTTP routes.
package handler

import (
	"net/http"

	"github.com/ShotaKitazawa/glidepath/internal/database/sqlcgen"
	"github.com/ShotaKitazawa/glidepath/internal/web"
)

// Register wires all routes onto mux.
//
// Routes follow SPEC.md's priority order (B > A > D > C/E), not the DB
// schema: GET / is the live forecast (B, the product itself — see
// home.go); /inventory is the one recurring task (A, 今月の記録);
// /assumptions ("設定") groups the rarely-changed inputs (family, big
// purchases, NISA funds, bank accounts) that feed the forecast but aren't
// checked day to day.
func Register(mux *http.ServeMux, queries *sqlcgen.Queries) {
	mux.HandleFunc("GET /healthz", healthz)
	web.RegisterStatic(mux)
	registerHome(mux, queries)
	registerInventory(mux, queries)
	registerAssumptions(mux, queries)
	registerFamily(mux, queries)
	registerBigPurchases(mux, queries)
	registerFunds(mux, queries)
	registerBankAccounts(mux, queries)
}

func healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
