package handler

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/ShotaKitazawa/glidepath/internal/database/sqlcgen"
)

// registerBankAccounts wires bank account registration. Accounts are
// displayed on GET /assumptions (registerAssumptions), not their own page
// — a rarely-changed input (SPEC.md 6章: accounts are stable entities you
// register once, unlike expense labels which vary freely month to month).
// Each month's per-account balance, by contrast, lives on POST /inventory
// (inventory.go), not here — same split as funds vs NISA拠出.
func registerBankAccounts(mux *http.ServeMux, q *sqlcgen.Queries) {
	mux.HandleFunc("POST /bank-accounts", bankAccountCreate(q))
	mux.HandleFunc("POST /bank-accounts/{id}/delete", bankAccountDelete(q))
}

type bankAccountView struct {
	ID                       int32
	Name                     string
	LatestBalanceThousandYen int32
	HasLatestBalance         bool
}

// loadBankAccountsView reads all bank accounts for display on
// GET /assumptions.
func loadBankAccountsView(ctx context.Context, q *sqlcgen.Queries) ([]bankAccountView, error) {
	accounts, err := q.ListBankAccounts(ctx)
	if err != nil {
		return nil, err
	}

	views := make([]bankAccountView, 0, len(accounts))
	for _, a := range accounts {
		view := bankAccountView{ID: a.ID, Name: a.Name}
		if latest, err := q.GetLatestBankAccountBalance(ctx, a.ID); err == nil {
			view.LatestBalanceThousandYen = toThousandYen(int(latest.Amount))
			view.HasLatestBalance = true
		}
		views = append(views, view)
	}
	return views, nil
}

func bankAccountCreate(q *sqlcgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		name := r.FormValue("account_name")
		if name == "" {
			renderAssumptionsError(w, r.Context(), q, "口座名を入力してください")
			return
		}

		if _, err := q.CreateBankAccount(r.Context(), name); err != nil {
			renderAssumptionsError(w, r.Context(), q, fmt.Sprintf("保存に失敗しました: %v", err))
			return
		}

		http.Redirect(w, r, "/assumptions", http.StatusSeeOther)
	}
}

func bankAccountDelete(q *sqlcgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		if err := q.DeleteBankAccount(r.Context(), int32(id)); err != nil {
			renderAssumptionsError(w, r.Context(), q, fmt.Sprintf("削除に失敗しました: %v", err))
			return
		}
		http.Redirect(w, r, "/assumptions", http.StatusSeeOther)
	}
}
