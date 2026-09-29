package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/ShotaKitazawa/glidepath/internal/database/sqlcgen"
	"github.com/ShotaKitazawa/glidepath/internal/web"
)

// registerAssumptions wires GET /assumptions ("設定"): family/education,
// big purchases, NISA funds, and bank accounts shown as one page. These
// are inputs the user sets up once and rarely revisits — grouping them
// keeps them out of the way of the two things actually done regularly
// (今月の記録 and 見通し). ?edit_big_purchase={id} switches the 周期的大型出費
// form into edit mode for that row (see bigPurchaseUpdate).
func registerAssumptions(mux *http.ServeMux, q *sqlcgen.Queries) {
	mux.HandleFunc("GET /assumptions", func(w http.ResponseWriter, r *http.Request) {
		var editBigPurchaseID int32
		if s := r.URL.Query().Get("edit_big_purchase"); s != "" {
			if id, err := strconv.Atoi(s); err == nil {
				editBigPurchaseID = int32(id)
			}
		}
		renderAssumptions(w, r.Context(), q, "", editBigPurchaseID)
	})
}

type assumptionsPageData struct {
	Title           string
	Members         []familyMemberView
	Purchases       []bigPurchaseView
	Funds           []fundView
	BankAccounts    []bankAccountView
	Error           string
	EditingPurchase *bigPurchaseView
}

func renderAssumptions(w http.ResponseWriter, ctx context.Context, q *sqlcgen.Queries, errMsg string, editBigPurchaseID int32) {
	members, err := loadFamilyView(ctx, q)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	purchases, err := loadBigPurchasesView(ctx, q)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	funds, err := loadFundsView(ctx, q)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	bankAccounts, err := loadBankAccountsView(ctx, q)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var editingPurchase *bigPurchaseView
	if editBigPurchaseID != 0 {
		if p, err := q.GetBigPurchase(ctx, editBigPurchaseID); err == nil {
			v := bigPurchaseRowToView(p)
			editingPurchase = &v
		}
	}

	status := http.StatusOK
	if errMsg != "" {
		status = http.StatusUnprocessableEntity
	}
	web.Render(w, status, "assumptions.html", assumptionsPageData{
		Title:           "設定",
		Members:         members,
		Purchases:       purchases,
		Funds:           funds,
		BankAccounts:    bankAccounts,
		Error:           errMsg,
		EditingPurchase: editingPurchase,
	})
}

// renderAssumptionsError is the shared error path for every mutation on
// this page (family, big purchases, funds, bank accounts) — it re-renders
// the full page with errMsg shown once at the top, rather than each domain
// keeping its own error rendering. It always drops back out of the
// 周期的大型出費 edit mode, consistent with how every other form on this page
// discards invalid input on error rather than preserving it.
func renderAssumptionsError(w http.ResponseWriter, ctx context.Context, q *sqlcgen.Queries, errMsg string) {
	renderAssumptions(w, ctx, q, errMsg, 0)
}
