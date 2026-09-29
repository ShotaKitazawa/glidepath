package handler

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ShotaKitazawa/glidepath/internal/calc"
	"github.com/ShotaKitazawa/glidepath/internal/database/sqlcgen"
	"github.com/ShotaKitazawa/glidepath/internal/web"
)

// recentIncomeMonths bounds how many past months' 収入 are offered as
// quick-fill buttons on the monthly form (SPEC.md 6章 note): one month back
// alone isn't enough, since a bonus month throws off a plain "same as last
// month" — three gives a decent chance one of them is a comparable month.
const recentIncomeMonths = 3

func registerInventory(mux *http.ServeMux, q *sqlcgen.Queries) {
	mux.HandleFunc("GET /inventory", func(w http.ResponseWriter, r *http.Request) {
		var successMsg string
		if r.URL.Query().Get("saved") == "1" {
			successMsg = "保存しました。"
		}
		renderInventory(w, r.Context(), q, r.URL.Query().Get("month"), successMsg, "")
	})
	mux.HandleFunc("POST /inventory", inventoryCreate(q))
	mux.HandleFunc("GET /inventory/history", inventoryHistory(q))
}

type inventoryRow struct {
	Month             string
	IncomeThousandYen int32
	BankBalance       int32
	ExpenseTotal      int32
}

// buildInventoryRows summarizes every monthly_records row (most recent
// first, per ListMonthlyRecords) — shared by the current-month form (which
// only needs the most recent one, for the "previous month" recall hint)
// and the standalone /inventory/history page (which lists all of them).
func buildInventoryRows(ctx context.Context, q *sqlcgen.Queries, records []sqlcgen.MonthlyRecord) ([]inventoryRow, error) {
	rows := make([]inventoryRow, 0, len(records))
	for _, rec := range records {
		cats, err := q.ListExpenseCategoriesByMonthlyRecord(ctx, rec.ID)
		if err != nil {
			return nil, err
		}
		var total int32
		for _, c := range cats {
			total += c.Amount
		}
		rows = append(rows, inventoryRow{
			Month:             rec.RecordMonth.Time.Format("2006-01"),
			IncomeThousandYen: toThousandYen(int(rec.IncomeMonthly)),
			BankBalance:       toThousandYen(int(rec.BankBalance)),
			ExpenseTotal:      toThousandYen(int(total)),
		})
	}
	return rows, nil
}

type inventoryHistoryPageData struct {
	Title   string
	Records []inventoryRow
}

// inventoryHistory lists every past month's record as a reference/lookup
// table, with a link into the current-month form (editing mode) for each
// one. Not folded into /inventory itself: that page stays the single-month
// form only.
func inventoryHistory(q *sqlcgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		records, err := q.ListMonthlyRecords(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		rows, err := buildInventoryRows(r.Context(), q, records)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		web.Render(w, http.StatusOK, "inventory_history.html", inventoryHistoryPageData{
			Title:   "実績",
			Records: rows,
		})
	}
}

// inventoryFundOption is a NISA fund offered on the monthly form.
// PrefillThousandYen carries the existing contribution for whichever month
// is being edited (0 when creating a new entry, or when there wasn't one).
type inventoryFundOption struct {
	ID                 int32
	Name               string
	PrefillThousandYen int32
	PrefillIsSpot      bool
}

// inventoryBankAccountOption is a registered bank account offered on the
// monthly form. PrefillThousandYen mirrors inventoryFundOption's.
type inventoryBankAccountOption struct {
	ID                 int32
	Name               string
	PrefillThousandYen int32
}

// recentIncome is one past month's 収入, offered as a quick-fill
// button on the monthly form.
type recentIncome struct {
	Month             string
	IncomeThousandYen int32
}

// expenseItemView is one labeled expense line item — used both for
// "previous month's items" (a recall aid) and for pre-filling the dynamic
// row form when editing an existing month.
type expenseItemView struct {
	Label       string
	ThousandYen int32
}

type inventoryPageData struct {
	Title                string
	Funds                []inventoryFundOption
	BankAccounts         []inventoryBankAccountOption
	RecentIncomes        []recentIncome
	PreviousExpenseMonth string
	PreviousExpenseItems []expenseItemView
	DefaultMonth         string
	// IsEditing is true when the form is pre-filled with an existing
	// month's data (GET /inventory?month=YYYY-MM), as opposed to starting
	// blank for a new entry.
	IsEditing                     bool
	PrefillIncomeThousandYen      int32
	PrefillBankBalanceThousandYen int32
	PrefillExpenseItems           []expenseItemView
	Success                       string
	Error                         string
}

func renderInventory(w http.ResponseWriter, ctx context.Context, q *sqlcgen.Queries, editMonth, successMsg, errMsg string) {
	records, err := q.ListMonthlyRecords(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var previousExpenseMonth string
	var previousExpenseItems []expenseItemView
	if len(records) > 0 {
		// records is ordered by record_month DESC, so records[0] is the
		// most recently logged month — reminds the user what they labeled
		// last time while they fill in this month's breakdown.
		cats, err := q.ListExpenseCategoriesByMonthlyRecord(ctx, records[0].ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		previousExpenseMonth = records[0].RecordMonth.Time.Format("2006-01")
		previousExpenseItems = make([]expenseItemView, 0, len(cats))
		for _, c := range cats {
			previousExpenseItems = append(previousExpenseItems, expenseItemView{
				Label:       c.Category,
				ThousandYen: toThousandYen(int(c.Amount)),
			})
		}
	}

	// records is ordered by record_month DESC (ListMonthlyRecords), so the
	// first recentIncomeMonths entries are exactly the most recent months.
	recentIncomes := make([]recentIncome, 0, recentIncomeMonths)
	for i, rec := range records {
		if i >= recentIncomeMonths {
			break
		}
		recentIncomes = append(recentIncomes, recentIncome{
			Month:             rec.RecordMonth.Time.Format("2006-01"),
			IncomeThousandYen: toThousandYen(int(rec.IncomeMonthly)),
		})
	}

	funds, err := q.ListFunds(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	accounts, err := q.ListBankAccounts(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// デフォルトは前月であり当月ではない: 実績は月が終わってから確定する
	// （9/1に入力するのは8月分）。
	targetMonth := time.Now().AddDate(0, -1, 0).Format("2006-01")

	if editMonth != "" {
		targetMonth = editMonth
	}

	// 対象月に既存レコードがあれば、?month= の明示指定なしでも自動的に
	// 編集状態にする。空欄に戻すための抜け道は用意しない — 抜け出した先の
	// 「空の状態」自体に意味がないため。他の月へは /inventory/history から。
	var editingRec *sqlcgen.MonthlyRecord
	for i := range records {
		if records[i].RecordMonth.Time.Format("2006-01") == targetMonth {
			editingRec = &records[i]
			break
		}
	}

	data := inventoryPageData{
		Title:                "今月の記録",
		RecentIncomes:        recentIncomes,
		PreviousExpenseMonth: previousExpenseMonth,
		PreviousExpenseItems: previousExpenseItems,
		DefaultMonth:         targetMonth,
		Success:              successMsg,
		Error:                errMsg,
	}

	if editingRec != nil {
		data.IsEditing = true
		data.PrefillIncomeThousandYen = toThousandYen(int(editingRec.IncomeMonthly))
		data.PrefillBankBalanceThousandYen = toThousandYen(int(editingRec.BankBalance))

		cats, err := q.ListExpenseCategoriesByMonthlyRecord(ctx, editingRec.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		data.PrefillExpenseItems = make([]expenseItemView, 0, len(cats))
		for _, c := range cats {
			data.PrefillExpenseItems = append(data.PrefillExpenseItems, expenseItemView{
				Label:       c.Category,
				ThousandYen: toThousandYen(int(c.Amount)),
			})
		}

		balances, err := q.ListBankAccountBalancesByMonthlyRecord(ctx, editingRec.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		balanceByAccount := make(map[int32]int32, len(balances))
		for _, b := range balances {
			balanceByAccount[b.BankAccountID] = toThousandYen(int(b.Amount))
		}

		data.BankAccounts = make([]inventoryBankAccountOption, 0, len(accounts))
		for _, a := range accounts {
			data.BankAccounts = append(data.BankAccounts, inventoryBankAccountOption{
				ID: a.ID, Name: a.Name, PrefillThousandYen: balanceByAccount[a.ID],
			})
		}

		data.Funds = make([]inventoryFundOption, 0, len(funds))
		for _, f := range funds {
			opt := inventoryFundOption{ID: f.ID, Name: f.Name}
			if contrib, err := q.GetNisaContributionByFundAndDate(ctx, sqlcgen.GetNisaContributionByFundAndDateParams{
				FundID:           f.ID,
				ContributionDate: editingRec.RecordMonth,
			}); err == nil {
				opt.PrefillThousandYen = toThousandYen(int(contrib.Amount))
				opt.PrefillIsSpot = contrib.ContributionType == string(calc.ContributionSpot)
			}
			data.Funds = append(data.Funds, opt)
		}
	} else {
		data.BankAccounts = make([]inventoryBankAccountOption, 0, len(accounts))
		for _, a := range accounts {
			data.BankAccounts = append(data.BankAccounts, inventoryBankAccountOption{ID: a.ID, Name: a.Name})
		}
		data.Funds = make([]inventoryFundOption, 0, len(funds))
		for _, f := range funds {
			data.Funds = append(data.Funds, inventoryFundOption{ID: f.ID, Name: f.Name})
		}
	}

	status := http.StatusOK
	if errMsg != "" {
		status = http.StatusUnprocessableEntity
	}
	web.Render(w, status, "inventory.html", data)
}

func inventoryCreate(q *sqlcgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		monthStr := r.FormValue("record_month")
		month, err := time.Parse("2006-01", monthStr)
		if err != nil {
			renderInventory(w, r.Context(), q, "", "", "対象月の形式が不正です")
			return
		}
		// re-render (on error) back to whatever month was being edited,
		// not always the default month.
		reRender := func(msg string) {
			renderInventory(w, r.Context(), q, monthStr, "", msg)
		}

		// 収入は毎月直接入力する（額面年収の12等分だと税・社会保険料の分だけ
		// 実際の入金額とズレるため、SPEC.md 6章の判断で年次更新から変更）。
		incomeMonthly, err := strconv.Atoi(r.FormValue("income_monthly"))
		if err != nil {
			reRender("収入の形式が不正です")
			return
		}
		// 口座は設定で一度登録する固定的な実体（NISAファンドと同じ構造）。
		// 登録済みなら口座ごとの残高を合算し、未登録なら（初回のブートストラップ
		// 用に）直接の合計入力にフォールバックする。
		accounts, err := q.ListBankAccounts(r.Context())
		if err != nil {
			reRender(fmt.Sprintf("口座情報の読み込みに失敗しました: %v", err))
			return
		}
		var bankBalanceThousand int
		accountBalances := make(map[int32]int) // bank_account_id -> 千円
		if len(accounts) > 0 {
			for _, a := range accounts {
				amountStr := r.FormValue(fmt.Sprintf("account_%d", a.ID))
				amount, err := strconv.Atoi(amountStr)
				if err != nil {
					reRender(fmt.Sprintf("%sの残高の形式が不正です", a.Name))
					return
				}
				accountBalances[a.ID] = amount
				bankBalanceThousand += amount
			}
		} else {
			bankBalanceThousand, err = strconv.Atoi(r.FormValue("bank_balance"))
			if err != nil {
				reRender("口座残高の形式が不正です")
				return
			}
		}

		rec, err := q.UpsertMonthlyRecord(r.Context(), sqlcgen.UpsertMonthlyRecordParams{
			RecordMonth:   pgtype.Date{Time: month, Valid: true},
			IncomeMonthly: fromThousandYen(incomeMonthly),
			BankBalance:   fromThousandYen(bankBalanceThousand),
		})
		if err != nil {
			reRender(fmt.Sprintf("保存に失敗しました: %v", err))
			return
		}

		if err := q.DeleteBankAccountBalancesByMonthlyRecord(r.Context(), rec.ID); err != nil {
			reRender(fmt.Sprintf("保存に失敗しました: %v", err))
			return
		}
		for accountID, amount := range accountBalances {
			if _, err := q.CreateBankAccountBalance(r.Context(), sqlcgen.CreateBankAccountBalanceParams{
				MonthlyRecordID: rec.ID,
				BankAccountID:   accountID,
				Amount:          fromThousandYen(amount),
			}); err != nil {
				reRender(fmt.Sprintf("保存に失敗しました: %v", err))
				return
			}
		}

		if err := q.DeleteExpenseCategoriesByMonthlyRecord(r.Context(), rec.ID); err != nil {
			reRender(fmt.Sprintf("保存に失敗しました: %v", err))
			return
		}
		// Each row of the dynamic "+ 項目を追加" list submits a pair of
		// same-named fields (expense_label[i], expense_amount[i]) — see
		// inventory.html. A labeled item gets its own expense_categories
		// row; this is what lets renderInventory remind the user next
		// month what they logged (the reason labels are stored at all).
		// No rows at all just means no expense recorded this month.
		items := parseExpenseItems(r.Form["expense_label"], r.Form["expense_amount"])
		for _, item := range items {
			if _, err := q.CreateExpenseCategory(r.Context(), sqlcgen.CreateExpenseCategoryParams{
				MonthlyRecordID: rec.ID,
				Category:        item.Label,
				Amount:          fromThousandYen(item.Amount),
			}); err != nil {
				reRender(fmt.Sprintf("保存に失敗しました: %v", err))
				return
			}
		}

		// NISA拠出も月次棚卸しの一部（SPEC.md 6章: 支出・NISA拠出・bank_balanceを
		// 月次入力）。登録済みファンドごとに拠出額を受け取る。同じ月を編集して
		// 再送信した時に重複しないよう、まず同じ月・同じファンドの既存拠出を消す。
		funds, err := q.ListFunds(r.Context())
		if err != nil {
			reRender(fmt.Sprintf("ファンド情報の読み込みに失敗しました: %v", err))
			return
		}
		for _, fund := range funds {
			if err := q.DeleteNisaContributionByFundAndDate(r.Context(), sqlcgen.DeleteNisaContributionByFundAndDateParams{
				FundID:           fund.ID,
				ContributionDate: pgtype.Date{Time: month, Valid: true},
			}); err != nil {
				reRender(fmt.Sprintf("NISA拠出の保存に失敗しました: %v", err))
				return
			}

			amountStr := r.FormValue(fmt.Sprintf("contribution_%d", fund.ID))
			if amountStr == "" {
				continue
			}
			amount, err := strconv.Atoi(amountStr)
			if err != nil || amount <= 0 {
				continue
			}
			contributionType := calc.ContributionRecurring
			if r.FormValue(fmt.Sprintf("contribution_spot_%d", fund.ID)) != "" {
				contributionType = calc.ContributionSpot
			}
			if _, err := q.CreateNisaContribution(r.Context(), sqlcgen.CreateNisaContributionParams{
				ContributionDate: pgtype.Date{Time: month, Valid: true},
				Amount:           fromThousandYen(amount),
				FundID:           fund.ID,
				ContributionType: string(contributionType),
			}); err != nil {
				reRender(fmt.Sprintf("NISA拠出の保存に失敗しました: %v", err))
				return
			}
		}

		// Not redirecting to 見通し: landing there with no acknowledgement
		// left it unclear whether the save had actually happened.
		http.Redirect(w, r, fmt.Sprintf("/inventory?month=%s&saved=1", monthStr), http.StatusSeeOther)
	}
}
