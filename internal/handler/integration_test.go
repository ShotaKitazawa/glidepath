//go:build integration

// Integration tests exercise the full HTTP stack (Register → real SQLite)
// end to end — the flow this package's manual curl-based verification kept
// covering by hand. Run via `mise run test-integration`, which points
// DATABASE_URL at a fresh scratch SQLite file so these runs never touch the
// file used for manual verification (see mise.toml).
//
// Excluded on purpose: anything that calls the real MUFG API
// (POST /funds/{id}/sync-nav) — network-dependent, slow, and unreliable to
// run on every test invocation. NAV history needed for the home/見通し
// assertions is inserted directly via sqlcgen instead.
package handler

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/ShotaKitazawa/glidepath/internal/calc"
	"github.com/ShotaKitazawa/glidepath/internal/database/sqlcgen"
)

func newIntegrationServer(t *testing.T) (*httptest.Server, *sqlcgen.Queries) {
	t.Helper()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Fatal("DATABASE_URL must be set (run via `mise run test-integration`)")
	}

	db, err := sql.Open("sqlite", dbURL)
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := db.PingContext(context.Background()); err != nil {
		t.Fatalf("pinging database: %v", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enabling foreign keys: %v", err)
	}

	q := sqlcgen.New(db)

	mux := http.NewServeMux()
	Register(mux, q)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return srv, q
}

// postForm submits a application/x-www-form-urlencoded POST and returns the
// final response after following redirects (net/http's default client
// behavior), so callers can assert on the landing page's content.
func postForm(t *testing.T, client *http.Client, rawURL string, values url.Values) *http.Response {
	t.Helper()
	resp, err := client.PostForm(rawURL, values)
	if err != nil {
		t.Fatalf("POST %s: %v", rawURL, err)
	}
	return resp
}

func bodyString(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading response body: %v", err)
	}
	return string(b)
}

// TestIntegration_FullFlow walks the same sequence a new user would: save
// this month's record (収入 entered directly, expense total, NISA
// contribution), register the rarely-changed assumptions (family, big
// purchase, fund), and see 見通し go from a guidance message to a live
// forecast once NAV history exists.
func TestIntegration_FullFlow(t *testing.T) {
	srv, q := newIntegrationServer(t)
	client := srv.Client()

	t.Run("home shows guidance before any data exists", func(t *testing.T) {
		resp, err := client.Get(srv.URL + "/")
		if err != nil {
			t.Fatalf("GET /: %v", err)
		}
		body := bodyString(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200; body:\n%s", resp.StatusCode, body)
		}
		if !strings.Contains(body, "今月の記録を入力してください") {
			t.Errorf("expected guidance to mention 今月の記録, got:\n%s", body)
		}
	})

	t.Run("monthly save takes 収入 directly (in 千円) and labeled expense items", func(t *testing.T) {
		resp := postForm(t, client, srv.URL+"/inventory", url.Values{
			"record_month":   {"2026-08"},
			"income_monthly": {"500"},  // 500千円 -> 500,000円
			"bank_balance":   {"3000"}, // 3000千円 -> 3,000,000円
			"expense_label":  {"食費", "日用品", "交通費"},
			"expense_amount": {"150", "80", "50"},
		})
		body := bodyString(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200; body:\n%s", resp.StatusCode, body)
		}
		if resp.Request.URL.Path != "/inventory" {
			t.Errorf("expected the save to land back on /inventory (with a success flash), got %s", resp.Request.URL.Path)
		}
		if !strings.Contains(body, "保存しました") {
			t.Errorf("expected a success flash after saving, got:\n%s", body)
		}

		ctx := context.Background()
		records, err := q.ListMonthlyRecords(ctx)
		if err != nil {
			t.Fatalf("ListMonthlyRecords: %v", err)
		}
		var rec *sqlcgen.MonthlyRecord
		for i := range records {
			if records[i].RecordMonth.Format("2006-01") == "2026-08" {
				rec = &records[i]
			}
		}
		if rec == nil {
			t.Fatal("2026-08 monthly record not found after save")
		}
		if rec.IncomeMonthly != 500000 {
			t.Errorf("income_monthly = %d, want 500000", rec.IncomeMonthly)
		}
		if rec.BankBalance != 3000000 {
			t.Errorf("bank_balance = %d, want 3000000", rec.BankBalance)
		}

		cats, err := q.ListExpenseCategoriesByMonthlyRecord(ctx, rec.ID)
		if err != nil {
			t.Fatalf("ListExpenseCategoriesByMonthlyRecord: %v", err)
		}
		want := map[string]int64{"食費": 150000, "日用品": 80000, "交通費": 50000}
		if len(cats) != len(want) {
			t.Fatalf("got %d expense_categories rows, want %d: %+v", len(cats), len(want), cats)
		}
		for _, c := range cats {
			if want[c.Category] != c.Amount {
				t.Errorf("category %q amount = %d, want %d", c.Category, c.Amount, want[c.Category])
			}
		}
		// Order must match submission order (食費, 日用品, 交通費), not
		// alphabetical — the form lets the user reorder rows before saving.
		wantOrder := []string{"食費", "日用品", "交通費"}
		for i, c := range cats {
			if c.Category != wantOrder[i] {
				t.Errorf("expense_categories order = %v, want %v", cats, wantOrder)
				break
			}
		}
	})

	t.Run("inventory page offers the saved month as a quick-fill button and recalls last month's labels", func(t *testing.T) {
		resp, err := client.Get(srv.URL + "/inventory")
		if err != nil {
			t.Fatalf("GET /inventory: %v", err)
		}
		body := bodyString(t, resp)
		if !strings.Contains(body, `data-income="500"`) {
			t.Errorf("expected a quick-fill button (千円) for the 2026-08 income, got:\n%s", body)
		}
		for _, want := range []string{"食費:150,000円", "日用品:80,000円", "交通費:50,000円"} {
			if !strings.Contains(body, want) {
				t.Errorf("expected last month's labeled breakdown to include %q, got:\n%s", want, body)
			}
		}
	})

	t.Run("assumptions: family member auto-generates education forecasts", func(t *testing.T) {
		resp := postForm(t, client, srv.URL+"/family", url.Values{
			"relation":         {"子"},
			"birth_month":      {"2018-04"},
			"track_pattern":    {"all_public"},
			"university_track": {"国公立"},
		})
		body := bodyString(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200; body:\n%s", resp.StatusCode, body)
		}
		if !strings.Contains(body, "子（2018-04生まれ）") {
			t.Errorf("expected the new family member to appear, got:\n%s", body)
		}
	})

	t.Run("assumptions: big purchase", func(t *testing.T) {
		resp := postForm(t, client, srv.URL+"/big-purchases", url.Values{
			"purchase_name":  {"車"},
			"base_amount":    {"3000"}, // 千円 -> 3,000,000円
			"base_date":      {"2024-06"},
			"cycle_years":    {"6"},
			"trade_in_value": {"200"}, // 千円 -> 200,000円
			"financing_mode": {"cash"},
			"recurring":      {"on"},
		})
		body := bodyString(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200; body:\n%s", resp.StatusCode, body)
		}
		if !strings.Contains(body, "車") {
			t.Errorf("expected the new big purchase to appear, got:\n%s", body)
		}
	})

	var bigPurchaseID int64
	t.Run("assumptions: big purchase edit prefills the form and update persists all fields", func(t *testing.T) {
		purchases, err := q.ListBigPurchases(context.Background())
		if err != nil || len(purchases) == 0 {
			t.Fatalf("ListBigPurchases: %v (len=%d)", err, len(purchases))
		}
		bigPurchaseID = purchases[0].ID

		editResp, err := client.Get(fmt.Sprintf("%s/assumptions?edit_big_purchase=%d", srv.URL, bigPurchaseID))
		if err != nil {
			t.Fatalf("GET /assumptions?edit_big_purchase=: %v", err)
		}
		editBody := bodyString(t, editResp)
		if editResp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200; body:\n%s", editResp.StatusCode, editBody)
		}
		if !strings.Contains(editBody, `value="3000"`) {
			t.Errorf("expected the edit form to be prefilled with the existing purchase's base_amount, got:\n%s", editBody)
		}
		if !strings.Contains(editBody, ">更新<") {
			t.Errorf("expected the submit button to say 更新 in edit mode, got:\n%s", editBody)
		}

		resp := postForm(t, client, fmt.Sprintf("%s/big-purchases/%d", srv.URL, bigPurchaseID), url.Values{
			"purchase_name":  {"バイク"},
			"base_amount":    {"500"}, // 千円 -> 500,000円
			"base_date":      {"2025-01"},
			"cycle_years":    {"8"},
			"trade_in_value": {"50"},
			"financing_mode": {"loan_if_favorable"},
		})
		body := bodyString(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200; body:\n%s", resp.StatusCode, body)
		}
		if !strings.Contains(body, "バイク") {
			t.Errorf("expected the renamed purchase to appear, got:\n%s", body)
		}

		updated, err := q.GetBigPurchase(context.Background(), bigPurchaseID)
		if err != nil {
			t.Fatalf("GetBigPurchase: %v", err)
		}
		if updated.Name != "バイク" || updated.BaseAmount != 500_000 || updated.CycleYears != 8 || updated.FinancingMode != "loan_if_favorable" {
			t.Errorf("updated purchase = %+v, fields did not all persist as submitted", updated)
		}
	})

	var fundID int64
	t.Run("assumptions: NISA fund", func(t *testing.T) {
		resp := postForm(t, client, srv.URL+"/funds", url.Values{
			"fund_name": {"eMAXIS Slim 米国株式（S&P500）"},
			"fund_code": {"253266"},
		})
		body := bodyString(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200; body:\n%s", resp.StatusCode, body)
		}

		funds, err := q.ListFunds(context.Background())
		if err != nil {
			t.Fatalf("ListFunds: %v", err)
		}
		if len(funds) == 0 {
			t.Fatal("no funds found after creating one")
		}
		fundID = funds[len(funds)-1].ID
	})

	t.Run("assumptions: NISA fund registered by URL alone extracts the fund code", func(t *testing.T) {
		resp := postForm(t, client, srv.URL+"/funds", url.Values{
			"fund_name":      {"テスト用ファンド"},
			"nav_source_url": {"https://www.am.mufg.jp/fund/999999.html"},
		})
		body := bodyString(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200; body:\n%s", resp.StatusCode, body)
		}

		funds, err := q.ListFunds(context.Background())
		if err != nil {
			t.Fatalf("ListFunds: %v", err)
		}
		var created *sqlcgen.Fund
		for i := range funds {
			if funds[i].Name == "テスト用ファンド" {
				created = &funds[i]
			}
		}
		if created == nil {
			t.Fatal("テスト用ファンド not found after creating it by URL alone")
		}
		if !created.IsinOrCode.Valid || created.IsinOrCode.String != "999999" {
			t.Errorf("fund_code = %+v, want extracted code 999999", created.IsinOrCode)
		}
		if !strings.Contains(body, "https://www.am.mufg.jp/fund/999999.html") {
			t.Errorf("expected the saved nav_source_url to be displayed on /assumptions, got:\n%s", body)
		}

		delResp := postForm(t, client, fmt.Sprintf("%s/funds/%d/delete", srv.URL, created.ID), url.Values{})
		delBody := bodyString(t, delResp)
		if delResp.StatusCode != http.StatusOK {
			t.Fatalf("delete status = %d, want 200; body:\n%s", delResp.StatusCode, delBody)
		}
		if strings.Contains(delBody, "テスト用ファンド") {
			t.Errorf("expected テスト用ファンド to be gone after delete, got:\n%s", delBody)
		}
		remaining, err := q.ListFunds(context.Background())
		if err != nil {
			t.Fatalf("ListFunds after delete: %v", err)
		}
		for _, f := range remaining {
			if f.ID == created.ID {
				t.Errorf("fund id %d still present after delete", created.ID)
			}
		}
	})

	var accountID int64
	t.Run("assumptions: bank account", func(t *testing.T) {
		resp := postForm(t, client, srv.URL+"/bank-accounts", url.Values{
			"account_name": {"個人口座"},
		})
		body := bodyString(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200; body:\n%s", resp.StatusCode, body)
		}
		if !strings.Contains(body, "個人口座") {
			t.Errorf("expected the new bank account to appear, got:\n%s", body)
		}

		accounts, err := q.ListBankAccounts(context.Background())
		if err != nil {
			t.Fatalf("ListBankAccounts: %v", err)
		}
		if len(accounts) == 0 {
			t.Fatal("no bank accounts found after creating one")
		}
		accountID = accounts[len(accounts)-1].ID
	})

	t.Run("GET /inventory?month= prefills the existing record for editing", func(t *testing.T) {
		resp, err := client.Get(srv.URL + "/inventory?month=2026-08")
		if err != nil {
			t.Fatalf("GET /inventory?month=2026-08: %v", err)
		}
		body := bodyString(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200; body:\n%s", resp.StatusCode, body)
		}
		if !strings.Contains(body, "2026-08の記録を編集しています") {
			t.Errorf("expected the edit banner for 2026-08, got:\n%s", body)
		}
		if !strings.Contains(body, `id="income_monthly" name="income_monthly" min="0" value="500"`) {
			t.Errorf("expected income_monthly to be prefilled with 500, got:\n%s", body)
		}
		for _, want := range []string{`value="食費"`, `value="150"`, `value="日用品"`, `value="80"`, `value="交通費"`, `value="50"`} {
			if !strings.Contains(body, want) {
				t.Errorf("expected prefilled expense item field %q, got:\n%s", want, body)
			}
		}
	})

	t.Run("resubmitting an edited month does not duplicate the NISA contribution", func(t *testing.T) {
		ctx := context.Background()
		contribField := fmt.Sprintf("contribution_%d", fundID)

		resp := postForm(t, client, srv.URL+"/inventory", url.Values{
			"record_month":                       {"2026-08"},
			"income_monthly":                     {"500"},
			fmt.Sprintf("account_%d", accountID): {"3000"},
			"expense_label":                      {"食費", "日用品", "交通費"},
			"expense_amount":                     {"150", "80", "50"},
			contribField:                         {"50"},
		})
		body := bodyString(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200; body:\n%s", resp.StatusCode, body)
		}

		resp2 := postForm(t, client, srv.URL+"/inventory", url.Values{
			"record_month":                       {"2026-08"},
			"income_monthly":                     {"500"},
			fmt.Sprintf("account_%d", accountID): {"3000"},
			"expense_label":                      {"食費", "日用品", "交通費"},
			"expense_amount":                     {"150", "80", "50"},
			contribField:                         {"80"},
		})
		body2 := bodyString(t, resp2)
		if resp2.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200; body:\n%s", resp2.StatusCode, body2)
		}

		contributions, err := q.ListNisaContributionsByFund(ctx, fundID)
		if err != nil {
			t.Fatalf("ListNisaContributionsByFund: %v", err)
		}
		var forMonth []sqlcgen.NisaContribution
		for _, c := range contributions {
			if c.ContributionDate.Format("2006-01") == "2026-08" {
				forMonth = append(forMonth, c)
			}
		}
		if len(forMonth) != 1 {
			t.Fatalf("got %d nisa_contributions rows for 2026-08, want 1 (resubmitting should replace, not duplicate): %+v", len(forMonth), forMonth)
		}
		if forMonth[0].Amount != 80000 {
			t.Errorf("nisa_contributions amount = %d, want 80000 (from the second submission)", forMonth[0].Amount)
		}
	})

	t.Run("monthly save takes a per-account balance once an account is registered", func(t *testing.T) {
		resp := postForm(t, client, srv.URL+"/inventory", url.Values{
			"record_month":                       {"2026-09"},
			"income_monthly":                     {"500"},
			fmt.Sprintf("account_%d", accountID): {"3200"}, // 3200千円 -> 3,200,000円
			"expense_label":                      {"食費"},
			"expense_amount":                     {"140"},
		})
		body := bodyString(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200; body:\n%s", resp.StatusCode, body)
		}

		ctx := context.Background()
		records, err := q.ListMonthlyRecords(ctx)
		if err != nil {
			t.Fatalf("ListMonthlyRecords: %v", err)
		}
		var rec *sqlcgen.MonthlyRecord
		for i := range records {
			if records[i].RecordMonth.Format("2006-01") == "2026-09" {
				rec = &records[i]
			}
		}
		if rec == nil {
			t.Fatal("2026-09 monthly record not found after save")
		}
		if rec.BankBalance != 3200000 {
			t.Errorf("bank_balance = %d, want 3200000 (from the single registered account)", rec.BankBalance)
		}

		balances, err := q.ListBankAccountBalancesByMonthlyRecord(ctx, rec.ID)
		if err != nil {
			t.Fatalf("ListBankAccountBalancesByMonthlyRecord: %v", err)
		}
		if len(balances) != 1 || balances[0].BankAccountID != accountID || balances[0].Amount != 3200000 {
			t.Errorf("bank_account_balances = %+v, want one row for account %d with amount 3200000", balances, accountID)
		}
	})

	t.Run("assumptions page shows the account's latest balance", func(t *testing.T) {
		resp, err := client.Get(srv.URL + "/assumptions")
		if err != nil {
			t.Fatalf("GET /assumptions: %v", err)
		}
		body := bodyString(t, resp)
		if !strings.Contains(body, "個人口座") || !strings.Contains(body, "3,200,000") {
			t.Errorf("expected 個人口座 with latest balance 3,200,000円, got:\n%s", body)
		}
	})

	t.Run("home shows a forecast even without any NISA NAV history", func(t *testing.T) {
		resp, err := client.Get(srv.URL + "/")
		if err != nil {
			t.Fatalf("GET /: %v", err)
		}
		body := bodyString(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200; body:\n%s", resp.StatusCode, body)
		}
		if !strings.Contains(body, "fan-chart") {
			t.Errorf("expected the fan chart to render without any NISA fund data, got:\n%s", body)
		}
		if strings.Contains(body, "先に") {
			t.Errorf("did not expect guidance text once a monthly record exists, got:\n%s", body)
		}
	})

	t.Run("home shows a live forecast once NAV history and a contribution exist", func(t *testing.T) {
		ctx := context.Background()
		base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		price := 30000
		for i := range 400 {
			d := base.AddDate(0, 0, i)
			if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
				continue
			}
			price += (i % 7) - 3 // small deterministic wiggle so returns aren't all zero
			if _, err := q.UpsertFundNavHistory(ctx, sqlcgen.UpsertFundNavHistoryParams{
				FundID:   fundID,
				NavDate:  d,
				NavPrice: int64(price),
			}); err != nil {
				t.Fatalf("UpsertFundNavHistory: %v", err)
			}
		}

		if _, err := q.CreateNisaContribution(ctx, sqlcgen.CreateNisaContributionParams{
			ContributionDate: base,
			Amount:           100000,
			FundID:           fundID,
			ContributionType: string(calc.ContributionRecurring),
		}); err != nil {
			t.Fatalf("CreateNisaContribution: %v", err)
		}

		resp, err := client.Get(srv.URL + "/")
		if err != nil {
			t.Fatalf("GET /: %v", err)
		}
		body := bodyString(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200; body:\n%s", resp.StatusCode, body)
		}
		if !strings.Contains(body, "fan-chart") {
			t.Errorf("expected the fan chart to render once prerequisites are met, got:\n%s", body)
		}
		if strings.Contains(body, "先に") {
			t.Errorf("did not expect guidance text once prerequisites are met, got:\n%s", body)
		}
	})

	t.Run("fund initial holding records a one-off nisa_contributions row independent of monthly_records", func(t *testing.T) {
		ctx := context.Background()
		asOf := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC) // within the NAV history synced above

		resp := postForm(t, client, fmt.Sprintf("%s/funds/%d/initial-holding", srv.URL, fundID), url.Values{
			"as_of_date": {"2025-06-01"},
			"amount":     {"500"}, // 500千円 -> 500,000円
		})
		body := bodyString(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200; body:\n%s", resp.StatusCode, body)
		}

		contrib, err := q.GetNisaContributionByFundAndDate(ctx, sqlcgen.GetNisaContributionByFundAndDateParams{
			FundID:           fundID,
			ContributionDate: asOf,
		})
		if err != nil {
			t.Fatalf("GetNisaContributionByFundAndDate: %v", err)
		}
		if contrib.Amount != 500000 {
			t.Errorf("amount = %d, want 500000", contrib.Amount)
		}

		records, err := q.ListMonthlyRecords(ctx)
		if err != nil {
			t.Fatalf("ListMonthlyRecords: %v", err)
		}
		for _, rec := range records {
			if rec.RecordMonth.Format("2006-01") == "2025-06" {
				t.Errorf("did not expect a monthly_records row for 2025-06 as a side effect, got %+v", rec)
			}
		}

		// Resubmitting the same date should replace, not duplicate.
		postForm(t, client, fmt.Sprintf("%s/funds/%d/initial-holding", srv.URL, fundID), url.Values{
			"as_of_date": {"2025-06-01"},
			"amount":     {"800"},
		})
		updated, err := q.GetNisaContributionByFundAndDate(ctx, sqlcgen.GetNisaContributionByFundAndDateParams{
			FundID:           fundID,
			ContributionDate: asOf,
		})
		if err != nil {
			t.Fatalf("GetNisaContributionByFundAndDate after resubmit: %v", err)
		}
		if updated.Amount != 800000 {
			t.Errorf("amount after resubmit = %d, want 800000 (replaced, not duplicated)", updated.Amount)
		}
	})

	t.Run("fund initial holding rejects a date before any synced NAV history", func(t *testing.T) {
		resp := postForm(t, client, fmt.Sprintf("%s/funds/%d/initial-holding", srv.URL, fundID), url.Values{
			"as_of_date": {"1999-01-01"},
			"amount":     {"500"},
		})
		body := bodyString(t, resp)
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422; body:\n%s", resp.StatusCode, body)
		}
		if !strings.Contains(body, "基準価額履歴がありません") {
			t.Errorf("expected a NAV-coverage error message, got:\n%s", body)
		}
	})

	var proxiedFundID int64
	t.Run("a fund can proxy another fund's NAV history instead of syncing its own", func(t *testing.T) {
		ctx := context.Background()

		resp := postForm(t, client, srv.URL+"/funds", url.Values{
			"fund_name":         {"(つみN) iFree S&P500インデックス"},
			"nav_proxy_fund_id": {fmt.Sprintf("%d", fundID)},
		})
		body := bodyString(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200; body:\n%s", resp.StatusCode, body)
		}
		if !strings.Contains(body, "値動き参照先: eMAXIS Slim") {
			t.Errorf("expected the proxy relationship to be shown, got:\n%s", body)
		}

		funds, err := q.ListFunds(ctx)
		if err != nil {
			t.Fatalf("ListFunds: %v", err)
		}
		var proxied *sqlcgen.Fund
		for i := range funds {
			if funds[i].Name == "(つみN) iFree S&P500インデックス" {
				proxied = &funds[i]
			}
		}
		if proxied == nil {
			t.Fatal("iFree fund not found after creating it")
		}
		if !proxied.NavProxyFundID.Valid || proxied.NavProxyFundID.Int64 != fundID {
			t.Errorf("nav_proxy_fund_id = %+v, want %d", proxied.NavProxyFundID, fundID)
		}
		if proxied.IsinOrCode.Valid {
			t.Errorf("expected no isin_or_code on a proxied fund, got %+v", proxied.IsinOrCode)
		}
		proxiedFundID = proxied.ID

		// It has none of its own fund_nav_history, but should still be
		// priceable via the proxy — within the NAV window synced earlier
		// (2025-01-01 .. ~2026-02-04).
		initResp := postForm(t, client, fmt.Sprintf("%s/funds/%d/initial-holding", srv.URL, proxiedFundID), url.Values{
			"as_of_date": {"2025-08-01"},
			"amount":     {"300"}, // 300千円 -> 300,000円
		})
		initBody := bodyString(t, initResp)
		if initResp.StatusCode != http.StatusOK {
			t.Fatalf("initial-holding status = %d, want 200; body:\n%s", initResp.StatusCode, initBody)
		}

		homeResp, err := client.Get(srv.URL + "/")
		if err != nil {
			t.Fatalf("GET /: %v", err)
		}
		homeBody := bodyString(t, homeResp)
		if homeResp.StatusCode != http.StatusOK {
			t.Fatalf("home status = %d, want 200; body:\n%s", homeResp.StatusCode, homeBody)
		}
		if !strings.Contains(homeBody, "fan-chart") {
			t.Errorf("expected the fan chart to still render with a proxied fund present, got:\n%s", homeBody)
		}
	})

	t.Run("sync-nav is refused for a fund that proxies another fund", func(t *testing.T) {
		resp := postForm(t, client, fmt.Sprintf("%s/funds/%d/sync-nav", srv.URL, proxiedFundID), url.Values{})
		body := bodyString(t, resp)
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422; body:\n%s", resp.StatusCode, body)
		}
		if !strings.Contains(body, "値動きを参照する設定") {
			t.Errorf("expected a proxy-aware refusal message, got:\n%s", body)
		}
	})
}

// TestIntegration_RouteWiring is a light sanity check that every page in
// the real Register() wiring responds, distinct from
// TestRegister_NoRoutePatternConflicts (which never issues real requests).
func TestIntegration_RouteWiring(t *testing.T) {
	srv, _ := newIntegrationServer(t)
	client := srv.Client()

	for _, path := range []string{"/healthz", "/", "/inventory", "/inventory/history", "/assumptions"} {
		resp, err := client.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s: status = %d, want 200", path, resp.StatusCode)
		}
	}
}
