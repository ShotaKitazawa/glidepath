package handler

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ShotaKitazawa/glidepath/internal/database/sqlcgen"
)

// registerBigPurchases wires the big-purchase mutations. Big purchases are
// displayed on GET /assumptions (registerAssumptions), not their own page —
// a rarely-changed input, not something checked day to day.
func registerBigPurchases(mux *http.ServeMux, q *sqlcgen.Queries) {
	mux.HandleFunc("POST /big-purchases", bigPurchaseCreate(q))
	mux.HandleFunc("POST /big-purchases/{id}", bigPurchaseUpdate(q))
	mux.HandleFunc("POST /big-purchases/{id}/growth-rate", bigPurchaseGrowthRateOverride(q))
	mux.HandleFunc("POST /big-purchases/{id}/delete", bigPurchaseDelete(q))
}

type bigPurchaseView struct {
	ID            int32
	Name          string
	BaseAmount    int32
	BaseDate      string
	CycleYears    int32
	GrowthRate    string
	TradeInValue  int32
	Recurring     bool
	FinancingMode string
}

// loadBigPurchasesView reads all big purchases for display on
// GET /assumptions.
func loadBigPurchasesView(ctx context.Context, q *sqlcgen.Queries) ([]bigPurchaseView, error) {
	purchases, err := q.ListBigPurchases(ctx)
	if err != nil {
		return nil, err
	}

	views := make([]bigPurchaseView, 0, len(purchases))
	for _, p := range purchases {
		views = append(views, bigPurchaseRowToView(p))
	}
	return views, nil
}

func bigPurchaseRowToView(p sqlcgen.BigPurchase) bigPurchaseView {
	rate := ""
	if p.CategoryGrowthRate.Valid {
		f, err := p.CategoryGrowthRate.Float64Value()
		if err == nil && f.Valid {
			rate = strconv.FormatFloat(f.Float64, 'f', -1, 64)
		}
	}
	return bigPurchaseView{
		ID:            p.ID,
		Name:          p.Name,
		BaseAmount:    toThousandYen(int(p.BaseAmount)),
		BaseDate:      p.BaseDate.Time.Format("2006-01"),
		CycleYears:    p.CycleYears,
		GrowthRate:    rate,
		TradeInValue:  toThousandYen(int(p.TradeInValue)),
		Recurring:     p.Recurring,
		FinancingMode: p.FinancingMode,
	}
}

// bigPurchaseFormInput is the fields shared by create and update.
// category_growth_rate is deliberately excluded — see
// bigPurchaseGrowthRateOverride, the only way to set it.
type bigPurchaseFormInput struct {
	Name          string
	BaseAmount    int
	BaseDate      time.Time
	CycleYears    int
	TradeInValue  int
	Recurring     bool
	FinancingMode string
}

func parseBigPurchaseForm(r *http.Request) (bigPurchaseFormInput, string) {
	name := r.FormValue("purchase_name")
	if name == "" {
		return bigPurchaseFormInput{}, "名称を入力してください"
	}
	baseAmount, err := strconv.Atoi(r.FormValue("base_amount"))
	if err != nil {
		return bigPurchaseFormInput{}, "基準金額の形式が不正です"
	}
	baseDate, err := time.Parse("2006-01", r.FormValue("base_date"))
	if err != nil {
		return bigPurchaseFormInput{}, "基準日の形式が不正です"
	}
	cycleYears, err := strconv.Atoi(r.FormValue("cycle_years"))
	if err != nil {
		return bigPurchaseFormInput{}, "周期年数の形式が不正です"
	}
	tradeInValue, err := strconv.Atoi(r.FormValue("trade_in_value"))
	if err != nil {
		tradeInValue = 0
	}
	financingMode := r.FormValue("financing_mode")
	if financingMode == "" {
		financingMode = "cash"
	}
	return bigPurchaseFormInput{
		Name:          name,
		BaseAmount:    baseAmount,
		BaseDate:      baseDate,
		CycleYears:    cycleYears,
		TradeInValue:  tradeInValue,
		Recurring:     r.FormValue("recurring") == "on",
		FinancingMode: financingMode,
	}, ""
}

func bigPurchaseCreate(q *sqlcgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		input, errMsg := parseBigPurchaseForm(r)
		if errMsg != "" {
			renderAssumptionsError(w, r.Context(), q, errMsg)
			return
		}

		if _, err := q.CreateBigPurchase(r.Context(), sqlcgen.CreateBigPurchaseParams{
			Name:          input.Name,
			BaseAmount:    fromThousandYen(input.BaseAmount),
			BaseDate:      pgtype.Date{Time: input.BaseDate, Valid: true},
			CycleYears:    int32(input.CycleYears),
			TradeInValue:  fromThousandYen(input.TradeInValue),
			Recurring:     input.Recurring,
			FinancingMode: input.FinancingMode,
		}); err != nil {
			renderAssumptionsError(w, r.Context(), q, fmt.Sprintf("保存に失敗しました: %v", err))
			return
		}

		http.Redirect(w, r, "/assumptions", http.StatusSeeOther)
	}
}

// bigPurchaseUpdate edits an existing purchase's fields (other than
// category_growth_rate — see bigPurchaseGrowthRateOverride). Reached from
// the same form as bigPurchaseCreate, switched into edit mode via
// GET /assumptions?edit_big_purchase={id}.
func bigPurchaseUpdate(q *sqlcgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		input, errMsg := parseBigPurchaseForm(r)
		if errMsg != "" {
			renderAssumptionsError(w, r.Context(), q, errMsg)
			return
		}

		if _, err := q.UpdateBigPurchase(r.Context(), sqlcgen.UpdateBigPurchaseParams{
			ID:            int32(id),
			Name:          input.Name,
			BaseAmount:    fromThousandYen(input.BaseAmount),
			BaseDate:      pgtype.Date{Time: input.BaseDate, Valid: true},
			CycleYears:    int32(input.CycleYears),
			TradeInValue:  fromThousandYen(input.TradeInValue),
			Recurring:     input.Recurring,
			FinancingMode: input.FinancingMode,
		}); err != nil {
			renderAssumptionsError(w, r.Context(), q, fmt.Sprintf("更新に失敗しました: %v", err))
			return
		}

		http.Redirect(w, r, "/assumptions", http.StatusSeeOther)
	}
}

// bigPurchaseGrowthRateOverride sets (or clears) a purchase's per-cycle
// value growth estimate after the fact — see bigPurchaseCreate for why it's
// not asked at registration time.
func bigPurchaseGrowthRateOverride(q *sqlcgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var growthRate pgtype.Numeric
		if s := r.FormValue("category_growth_rate"); s != "" {
			if err := growthRate.Scan(s); err != nil {
				renderAssumptionsError(w, r.Context(), q, "値上がり率の形式が不正です")
				return
			}
		}

		if _, err := q.UpdateBigPurchaseGrowthRate(r.Context(), sqlcgen.UpdateBigPurchaseGrowthRateParams{
			ID:                 int32(id),
			CategoryGrowthRate: growthRate,
		}); err != nil {
			renderAssumptionsError(w, r.Context(), q, fmt.Sprintf("更新に失敗しました: %v", err))
			return
		}
		http.Redirect(w, r, "/assumptions", http.StatusSeeOther)
	}
}

func bigPurchaseDelete(q *sqlcgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		if err := q.DeleteBigPurchase(r.Context(), int32(id)); err != nil {
			renderAssumptionsError(w, r.Context(), q, fmt.Sprintf("削除に失敗しました: %v", err))
			return
		}
		http.Redirect(w, r, "/assumptions", http.StatusSeeOther)
	}
}
