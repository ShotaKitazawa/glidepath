package handler

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ShotaKitazawa/glidepath/internal/calc"
	"github.com/ShotaKitazawa/glidepath/internal/database/sqlcgen"
	"github.com/ShotaKitazawa/glidepath/internal/mufg"
)

// mufgFundCodeInURL matches the fund code out of a MUFG fund page URL,
// e.g. https://www.am.mufg.jp/fund/253266.html -> "253266".
var mufgFundCodeInURL = regexp.MustCompile(`/fund/(\d+)`)

func extractFundCodeFromURL(rawURL string) (code string, ok bool) {
	m := mufgFundCodeInURL.FindStringSubmatch(rawURL)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// maxSyncWindowDays bounds how many days a single "基準価額を同期" click
// walks, so the request stays within a normal HTTP timeout. A brand-new
// fund with years of history needs several clicks (or the backfill-nav
// CLI, cmd/backfill-nav) to catch all the way up.
const maxSyncWindowDays = 400

// registerFunds wires the NISA fund mutations. Funds are displayed on
// GET /assumptions (registerAssumptions), not their own page — a rarely-
// changed input, not something checked day to day. Recording a
// contribution, by contrast, is monthly (SPEC.md 6章) and lives on
// POST /inventory (inventory.go), not here.
func registerFunds(mux *http.ServeMux, q *sqlcgen.Queries) {
	mux.HandleFunc("POST /funds", fundCreate(q))
	mux.HandleFunc("POST /funds/{id}/sync-nav", fundSyncNAV(q))
	mux.HandleFunc("POST /funds/{id}/initial-holding", fundInitialHolding(q))
	mux.HandleFunc("POST /funds/{id}/delete", fundDelete(q))
}

type fundView struct {
	ID            int32
	Name          string
	FundCode      string
	NavSourceURL  string
	LatestNAVDate string
	// LatestNAVYen stays raw yen (not 千円) — it's the fund's 基準価額 (a
	// per-10,000-unit price index, SPEC.md 3章), not a spendable amount.
	LatestNAVYen            int32
	TotalContribThousandYen int32
	// NavHistoryCount reflects this fund's own fund_nav_history rows (not
	// the proxy's, if any) — it backs the delete confirmation prompt, which
	// warns about exactly what that fund's own cascade delete would remove.
	NavHistoryCount int
	// NavProxyFundName is set when this fund has no NAV data of its own and
	// instead uses another fund's price history (SPEC.md 4.1) — empty
	// otherwise.
	NavProxyFundName string
}

// loadFundsView reads all NISA funds for display on GET /assumptions.
func loadFundsView(ctx context.Context, q *sqlcgen.Queries) ([]fundView, error) {
	funds, err := q.ListFunds(ctx)
	if err != nil {
		return nil, err
	}

	views := make([]fundView, 0, len(funds))
	for _, f := range funds {
		view := fundView{ID: f.ID, Name: f.Name}
		if f.IsinOrCode.Valid {
			view.FundCode = f.IsinOrCode.String
		}
		if f.NavSourceUrl.Valid {
			view.NavSourceURL = f.NavSourceUrl.String
		}

		ownNavRows, err := q.ListFundNavHistory(ctx, f.ID)
		if err != nil {
			return nil, err
		}
		view.NavHistoryCount = len(ownNavRows)

		navRows := ownNavRows
		if f.NavProxyFundID.Valid {
			proxyFund, err := q.GetFund(ctx, f.NavProxyFundID.Int32)
			if err != nil {
				return nil, err
			}
			view.NavProxyFundName = proxyFund.Name
			navRows, err = q.ListFundNavHistory(ctx, f.NavProxyFundID.Int32)
			if err != nil {
				return nil, err
			}
		}
		if len(navRows) > 0 {
			latest := navRows[len(navRows)-1]
			view.LatestNAVDate = latest.NavDate.Time.Format("2006-01-02")
			view.LatestNAVYen = latest.NavPrice
		}

		contribs, err := q.ListNisaContributionsByFund(ctx, f.ID)
		if err != nil {
			return nil, err
		}
		calcContribs := make([]calc.Contribution, len(contribs))
		for i, c := range contribs {
			calcContribs[i] = calc.Contribution{
				Date:      c.ContributionDate.Time,
				AmountYen: int(c.Amount),
				Kind:      calc.ContributionKind(c.ContributionType),
			}
		}
		view.TotalContribThousandYen = toThousandYen(calc.TotalContributedYen(calcContribs))

		views = append(views, view)
	}
	return views, nil
}

// fundNavHistory returns fund's own NAV history, or — if it has no data
// source of its own and instead proxies another fund's price movement
// (funds.nav_proxy_fund_id, SPEC.md 4.1) — that fund's history instead.
func fundNavHistory(ctx context.Context, q *sqlcgen.Queries, fund sqlcgen.Fund) ([]sqlcgen.FundNavHistory, error) {
	sourceID := fund.ID
	if fund.NavProxyFundID.Valid {
		sourceID = fund.NavProxyFundID.Int32
	}
	return q.ListFundNavHistory(ctx, sourceID)
}

func fundCreate(q *sqlcgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		name := r.FormValue("fund_name")
		if name == "" {
			renderAssumptionsError(w, r.Context(), q, "ファンド名を入力してください")
			return
		}

		var navProxyFundID pgtype.Int4
		if s := r.FormValue("nav_proxy_fund_id"); s != "" {
			id, err := strconv.Atoi(s)
			if err != nil {
				renderAssumptionsError(w, r.Context(), q, "参照先ファンドの形式が不正です")
				return
			}
			navProxyFundID = pgtype.Int4{Int32: int32(id), Valid: true}
		}

		// 自前の基準価額同期（URL/コード）と、他ファンドの値動きを参照する
		// 設定は併用しない（SPEC.md 4.1）。参照先が指定されていれば、
		// 自前のコード・URL入力は無視する。
		var navSourceURL, fundCode string
		if !navProxyFundID.Valid {
			navSourceURL = r.FormValue("nav_source_url")
			fundCode = r.FormValue("fund_code")
			if fundCode == "" {
				if code, ok := extractFundCodeFromURL(navSourceURL); ok {
					fundCode = code
				}
			}
		}

		if _, err := q.CreateFund(r.Context(), sqlcgen.CreateFundParams{
			Name:           name,
			IsinOrCode:     pgtype.Text{String: fundCode, Valid: fundCode != ""},
			NavSourceUrl:   pgtype.Text{String: navSourceURL, Valid: navSourceURL != ""},
			NavProxyFundID: navProxyFundID,
		}); err != nil {
			renderAssumptionsError(w, r.Context(), q, fmt.Sprintf("保存に失敗しました: %v", err))
			return
		}

		http.Redirect(w, r, "/assumptions", http.StatusSeeOther)
	}
}

// fundSyncNAV fetches missing 基準価額 from the MUFG API (SPEC.md 4.1) for
// the window since the fund's last known quote, up to maxSyncWindowDays.
func fundSyncNAV(q *sqlcgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		fundID, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}

		fund, err := q.GetFund(ctx, int32(fundID))
		if err != nil {
			renderAssumptionsError(w, ctx, q, fmt.Sprintf("ファンドが見つかりません: %v", err))
			return
		}
		if fund.NavProxyFundID.Valid {
			renderAssumptionsError(w, ctx, q, "このファンドは他のファンドの値動きを参照する設定になっており、基準価額の同期は不要です")
			return
		}
		if !fund.IsinOrCode.Valid || fund.IsinOrCode.String == "" {
			renderAssumptionsError(w, ctx, q, "このファンドにはMUFGファンドコードが設定されていません")
			return
		}

		from := time.Now().AddDate(0, 0, -maxSyncWindowDays)
		if latest, err := q.GetLatestFundNav(ctx, fund.ID); err == nil {
			from = latest.NavDate.Time.AddDate(0, 0, 1)
		}
		to := time.Now().AddDate(0, 0, -1) // yesterday: today's NAV may not be published yet

		if from.After(to) {
			http.Redirect(w, r, "/assumptions", http.StatusSeeOther)
			return
		}
		if to.Sub(from) > maxSyncWindowDays*24*time.Hour {
			from = to.AddDate(0, 0, -maxSyncWindowDays)
		}

		client := mufg.NewClient()
		quotes, err := mufg.SyncQuotes(ctx, client, fund.IsinOrCode.String, from, to, 200*time.Millisecond)
		if err != nil {
			renderAssumptionsError(w, ctx, q, fmt.Sprintf("基準価額の取得に失敗しました: %v", err))
			return
		}

		for _, quote := range quotes {
			if _, err := q.UpsertFundNavHistory(ctx, sqlcgen.UpsertFundNavHistoryParams{
				FundID:   fund.ID,
				NavDate:  pgtype.Date{Time: quote.Date, Valid: true},
				NavPrice: int32(quote.NAVYen),
			}); err != nil {
				renderAssumptionsError(w, ctx, q, fmt.Sprintf("基準価額の保存に失敗しました: %v", err))
				return
			}
		}

		http.Redirect(w, r, "/assumptions", http.StatusSeeOther)
	}
}

// fundInitialHolding records money the user already held in a fund before
// they started tracking it here, as a one-off nisa_contributions row dated
// in the past — the same mechanism monthly NISA拠出 uses, just entered once
// and independent of monthly_records (nisa_contributions has no FK to it).
// Requires NAV history back to that date; unlike inventoryCreate, this
// checks that up front instead of letting it silently zero out the fund's
// valuation later.
func fundInitialHolding(q *sqlcgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		fundID, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		asOf, err := time.Parse("2006-01-02", r.FormValue("as_of_date"))
		if err != nil {
			renderAssumptionsError(w, ctx, q, "日付の形式が不正です")
			return
		}
		amountThousand, err := strconv.Atoi(r.FormValue("amount"))
		if err != nil || amountThousand <= 0 {
			renderAssumptionsError(w, ctx, q, "評価額の形式が不正です")
			return
		}

		fund, err := q.GetFund(ctx, int32(fundID))
		if err != nil {
			renderAssumptionsError(w, ctx, q, fmt.Sprintf("ファンドが見つかりません: %v", err))
			return
		}
		navRows, err := fundNavHistory(ctx, q, fund)
		if err != nil {
			renderAssumptionsError(w, ctx, q, fmt.Sprintf("基準価額履歴の読み込みに失敗しました: %v", err))
			return
		}
		hasCoverage := false
		for _, n := range navRows {
			if !n.NavDate.Time.After(asOf) {
				hasCoverage = true
				break
			}
		}
		if !hasCoverage {
			renderAssumptionsError(w, ctx, q, "指定日以前の基準価額履歴がありません。先に「基準価額を同期」またはbackfill-navで基準価額履歴をその日まで遡らせてください")
			return
		}

		if err := q.DeleteNisaContributionByFundAndDate(ctx, sqlcgen.DeleteNisaContributionByFundAndDateParams{
			FundID:           int32(fundID),
			ContributionDate: pgtype.Date{Time: asOf, Valid: true},
		}); err != nil {
			renderAssumptionsError(w, ctx, q, fmt.Sprintf("保存に失敗しました: %v", err))
			return
		}
		if _, err := q.CreateNisaContribution(ctx, sqlcgen.CreateNisaContributionParams{
			ContributionDate: pgtype.Date{Time: asOf, Valid: true},
			Amount:           fromThousandYen(amountThousand),
			FundID:           int32(fundID),
			ContributionType: string(calc.ContributionSnapshot),
		}); err != nil {
			renderAssumptionsError(w, ctx, q, fmt.Sprintf("保存に失敗しました: %v", err))
			return
		}

		http.Redirect(w, r, "/assumptions", http.StatusSeeOther)
	}
}

func fundDelete(q *sqlcgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		if err := q.DeleteFund(r.Context(), int32(id)); err != nil {
			renderAssumptionsError(w, r.Context(), q, fmt.Sprintf("削除に失敗しました: %v", err))
			return
		}
		http.Redirect(w, r, "/assumptions", http.StatusSeeOther)
	}
}
