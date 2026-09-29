package web

import (
	"html/template"
	"net/http/httptest"
	"testing"
)

// TestRender_AllPagesParseAndExecute catches template syntax errors and
// field-name mismatches between handlers and templates without needing a
// database: it renders every page template with representative data and
// asserts the response is a 200 with a non-trivial body.
func TestRender_AllPagesParseAndExecute(t *testing.T) {
	type inventoryRow struct {
		Month                                        string
		IncomeThousandYen, BankBalance, ExpenseTotal int32
	}
	type forecastRow struct {
		ID, StartAge, EndAge, AnnualCost int32
		Stage, Track                     string
		IsOverride                       bool
	}
	type familyMember struct {
		ID        int32
		Relation  string
		Birth     string
		Forecasts []forecastRow
	}
	type bigPurchase struct {
		ID, BaseAmount, CycleYears, TradeInValue  int32
		Name, BaseDate, GrowthRate, FinancingMode string
		Recurring                                 bool
	}
	type simulationPoint struct{ Year, P10, P50, P90 int32 }
	type actualPoint struct{ Year, NetWorthYen int32 }
	type fund struct {
		ID                                    int32
		Name, FundCode, NavSourceURL          string
		LatestNAVDate                         string
		LatestNAVYen, TotalContribThousandYen int32
		NavHistoryCount                       int
		NavProxyFundName                      string
	}
	type fundOption struct {
		ID                 int32
		Name               string
		PrefillThousandYen int32
		PrefillIsSpot      bool
	}
	type recentIncome struct {
		Month             string
		IncomeThousandYen int32
	}
	type expenseItemView struct {
		Label       string
		ThousandYen int32
	}
	type bankAccountOption struct {
		ID                 int32
		Name               string
		PrefillThousandYen int32
	}
	type bankAccount struct {
		ID                       int32
		Name                     string
		LatestBalanceThousandYen int32
		HasLatestBalance         bool
	}

	cases := []struct {
		name string
		tmpl string
		data any
	}{
		{
			name: "inventory",
			tmpl: "inventory.html",
			data: struct {
				Title                         string
				Funds                         []fundOption
				BankAccounts                  []bankAccountOption
				RecentIncomes                 []recentIncome
				PreviousExpenseMonth          string
				PreviousExpenseItems          []expenseItemView
				DefaultMonth                  string
				IsEditing                     bool
				PrefillIncomeThousandYen      int32
				PrefillBankBalanceThousandYen int32
				PrefillExpenseItems           []expenseItemView
				Success                       string
				Error                         string
			}{
				Title:        "今月の記録",
				Funds:        []fundOption{{ID: 1, Name: "eMAXIS Slim 米国株式（S&P500）"}},
				BankAccounts: []bankAccountOption{{ID: 1, Name: "個人口座"}, {ID: 2, Name: "共用口座"}},
				RecentIncomes: []recentIncome{
					{Month: "2025-01", IncomeThousandYen: 500},
					{Month: "2024-12", IncomeThousandYen: 650},
				},
				PreviousExpenseMonth: "2025-01",
				PreviousExpenseItems: []expenseItemView{
					{Label: "食費", ThousandYen: 15},
					{Label: "日用品", ThousandYen: 32},
				},
				DefaultMonth: "2025-02",
				Success:      "保存しました。",
			},
		},
		{
			name: "inventory_editing",
			tmpl: "inventory.html",
			data: struct {
				Title                         string
				Funds                         []fundOption
				BankAccounts                  []bankAccountOption
				RecentIncomes                 []recentIncome
				PreviousExpenseMonth          string
				PreviousExpenseItems          []expenseItemView
				DefaultMonth                  string
				IsEditing                     bool
				PrefillIncomeThousandYen      int32
				PrefillBankBalanceThousandYen int32
				PrefillExpenseItems           []expenseItemView
				Success                       string
				Error                         string
			}{
				Title:                    "今月の記録",
				Funds:                    []fundOption{{ID: 1, Name: "eMAXIS Slim 米国株式（S&P500）", PrefillThousandYen: 20}},
				BankAccounts:             []bankAccountOption{{ID: 1, Name: "個人口座", PrefillThousandYen: 1000}},
				DefaultMonth:             "2025-01",
				IsEditing:                true,
				PrefillIncomeThousandYen: 500,
				PrefillExpenseItems: []expenseItemView{
					{Label: "食費", ThousandYen: 15},
				},
			},
		},
		{
			name: "inventory_empty",
			tmpl: "inventory.html",
			data: struct {
				Title                         string
				Funds                         []fundOption
				BankAccounts                  []bankAccountOption
				RecentIncomes                 []recentIncome
				PreviousExpenseMonth          string
				PreviousExpenseItems          []expenseItemView
				DefaultMonth                  string
				IsEditing                     bool
				PrefillIncomeThousandYen      int32
				PrefillBankBalanceThousandYen int32
				PrefillExpenseItems           []expenseItemView
				Success                       string
				Error                         string
			}{
				Title:        "今月の記録",
				DefaultMonth: "2025-01",
			},
		},
		{
			name: "inventory_history",
			tmpl: "inventory_history.html",
			data: struct {
				Title   string
				Records []inventoryRow
			}{
				Title:   "実績",
				Records: []inventoryRow{{Month: "2025-01", IncomeThousandYen: 500, BankBalance: 1000, ExpenseTotal: 300}},
			},
		},
		{
			name: "inventory_history_empty",
			tmpl: "inventory_history.html",
			data: struct {
				Title   string
				Records []inventoryRow
			}{Title: "実績"},
		},
		{
			name: "assumptions",
			tmpl: "assumptions.html",
			data: struct {
				Title           string
				Members         []familyMember
				Purchases       []bigPurchase
				Funds           []fund
				BankAccounts    []bankAccount
				Error           string
				EditingPurchase *bigPurchase
			}{
				Title: "設定",
				Members: []familyMember{{
					ID: 1, Relation: "子", Birth: "2018-04",
					Forecasts: []forecastRow{{ID: 1, Stage: "小学校", Track: "公立", StartAge: 6, EndAge: 11, AnnualCost: 336000}},
				}},
				Purchases: []bigPurchase{{
					ID: 1, Name: "車", BaseAmount: 3000000, BaseDate: "2024-06",
					CycleYears: 6, GrowthRate: "0.1", TradeInValue: 200000,
					Recurring: true, FinancingMode: "cash",
				}},
				Funds: []fund{
					{
						ID: 1, Name: "eMAXIS Slim 米国株式（S&P500）", FundCode: "253266",
						NavSourceURL:  "https://www.am.mufg.jp/fund/253266.html",
						LatestNAVDate: "2026-09-18", LatestNAVYen: 43966, TotalContribThousandYen: 1200,
						NavHistoryCount: 120,
					},
					{
						ID: 2, Name: "(つみN) iFree S&P500インデックス",
						LatestNAVDate: "2026-09-18", LatestNAVYen: 43966, TotalContribThousandYen: 300,
						NavHistoryCount: 0, NavProxyFundName: "eMAXIS Slim 米国株式（S&P500）",
					},
				},
				BankAccounts: []bankAccount{
					{ID: 1, Name: "個人口座", LatestBalanceThousandYen: 1000, HasLatestBalance: true},
					{ID: 2, Name: "共用口座", HasLatestBalance: false},
				},
			},
		},
		{
			name: "assumptions_empty",
			tmpl: "assumptions.html",
			data: struct {
				Title           string
				Members         []familyMember
				Purchases       []bigPurchase
				Funds           []fund
				BankAccounts    []bankAccount
				Error           string
				EditingPurchase *bigPurchase
			}{Title: "設定"},
		},
		{
			name: "assumptions_editing_purchase",
			tmpl: "assumptions.html",
			data: struct {
				Title           string
				Members         []familyMember
				Purchases       []bigPurchase
				Funds           []fund
				BankAccounts    []bankAccount
				Error           string
				EditingPurchase *bigPurchase
			}{
				Title: "設定",
				Purchases: []bigPurchase{{
					ID: 1, Name: "車", BaseAmount: 3000000, BaseDate: "2024-06",
					CycleYears: 6, GrowthRate: "0.1", TradeInValue: 200000,
					Recurring: true, FinancingMode: "cash",
				}},
				EditingPurchase: &bigPurchase{
					ID: 1, Name: "車", BaseAmount: 3000000, BaseDate: "2024-06",
					CycleYears: 6, GrowthRate: "0.1", TradeInValue: 200000,
					Recurring: true, FinancingMode: "cash",
				},
			},
		},
		{
			name: "home_with_results",
			tmpl: "home.html",
			data: struct {
				Title          string
				ActualPoints   []actualPoint
				ForecastPoints []simulationPoint
				ActualJSON     template.JS
				ForecastJSON   template.JS
				Guidance       string
			}{
				Title:          "見通し",
				ActualPoints:   []actualPoint{{Year: 2025, NetWorthYen: 1500000}},
				ForecastPoints: []simulationPoint{{Year: 2026, P10: 1000000, P50: 2000000, P90: 3000000}},
				ActualJSON:     template.JS(`[{"Year":2025,"NetWorthYen":1500000}]`),
				ForecastJSON:   template.JS(`[{"Year":2026,"P10":1000000,"P50":2000000,"P90":3000000}]`),
			},
		},
		{
			name: "home_guidance",
			tmpl: "home.html",
			data: struct {
				Title          string
				ActualPoints   []actualPoint
				ForecastPoints []simulationPoint
				ActualJSON     template.JS
				ForecastJSON   template.JS
				Guidance       string
			}{
				Title:    "見通し",
				Guidance: "先に今月の記録を入力してください",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			Render(rec, 200, tc.tmpl, tc.data)
			if rec.Code != 200 {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if rec.Body.Len() < 100 {
				t.Fatalf("body too short (%d bytes), template likely failed: %s", rec.Body.Len(), rec.Body.String())
			}
		})
	}
}
