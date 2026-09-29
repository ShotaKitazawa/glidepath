-- Queries are added here alongside schema.sql as each table is implemented.

-- monthly_records --------------------------------------------------------

-- name: UpsertMonthlyRecord :one
INSERT INTO monthly_records (record_month, income_monthly, bank_balance)
VALUES ($1, $2, $3)
ON CONFLICT (record_month)
DO UPDATE SET income_monthly = EXCLUDED.income_monthly, bank_balance = EXCLUDED.bank_balance
RETURNING *;

-- name: GetMonthlyRecordByMonth :one
SELECT * FROM monthly_records WHERE record_month = $1;

-- name: ListMonthlyRecords :many
SELECT * FROM monthly_records ORDER BY record_month DESC;

-- expense_categories ------------------------------------------------------

-- name: CreateExpenseCategory :one
INSERT INTO expense_categories (monthly_record_id, category, amount)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListExpenseCategoriesByMonthlyRecord :many
SELECT * FROM expense_categories WHERE monthly_record_id = $1 ORDER BY id;

-- name: ListExpenseCategories :many
SELECT ec.* FROM expense_categories ec
JOIN monthly_records mr ON mr.id = ec.monthly_record_id
ORDER BY mr.record_month;

-- name: DeleteExpenseCategoriesByMonthlyRecord :exec
DELETE FROM expense_categories WHERE monthly_record_id = $1;

-- family_members -----------------------------------------------------------

-- name: CreateFamilyMember :one
INSERT INTO family_members (relation, birth_month)
VALUES ($1, $2)
RETURNING *;

-- name: ListFamilyMembers :many
SELECT * FROM family_members ORDER BY birth_month;

-- name: GetFamilyMember :one
SELECT * FROM family_members WHERE id = $1;

-- name: DeleteFamilyMember :exec
DELETE FROM family_members WHERE id = $1;

-- expense_forecasts ---------------------------------------------------------

-- name: CreateExpenseForecast :one
INSERT INTO expense_forecasts (family_member_id, stage, track, annual_cost, start_age, end_age, is_override)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListExpenseForecastsByFamilyMember :many
SELECT * FROM expense_forecasts WHERE family_member_id = $1 ORDER BY start_age;

-- name: ListExpenseForecasts :many
SELECT * FROM expense_forecasts ORDER BY family_member_id, start_age;

-- name: UpdateExpenseForecastOverride :one
UPDATE expense_forecasts
SET annual_cost = $2, is_override = true
WHERE id = $1
RETURNING *;

-- name: DeleteExpenseForecast :exec
DELETE FROM expense_forecasts WHERE id = $1;

-- big_purchases --------------------------------------------------------------

-- name: CreateBigPurchase :one
INSERT INTO big_purchases (name, base_amount, base_date, cycle_years, category_growth_rate, trade_in_value, recurring, financing_mode)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: ListBigPurchases :many
SELECT * FROM big_purchases ORDER BY base_date;

-- name: GetBigPurchase :one
SELECT * FROM big_purchases WHERE id = $1;

-- name: UpdateBigPurchase :one
UPDATE big_purchases
SET name = $2, base_amount = $3, base_date = $4, cycle_years = $5,
    trade_in_value = $6, recurring = $7, financing_mode = $8
WHERE id = $1
RETURNING *;

-- name: UpdateBigPurchaseGrowthRate :one
UPDATE big_purchases
SET category_growth_rate = $2
WHERE id = $1
RETURNING *;

-- name: DeleteBigPurchase :exec
DELETE FROM big_purchases WHERE id = $1;

-- funds ------------------------------------------------------------------------

-- name: CreateFund :one
INSERT INTO funds (name, isin_or_code, nav_source_url, nav_proxy_fund_id)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListFunds :many
SELECT * FROM funds ORDER BY id;

-- name: GetFund :one
SELECT * FROM funds WHERE id = $1;

-- name: DeleteFund :exec
DELETE FROM funds WHERE id = $1;

-- fund_nav_history -----------------------------------------------------------

-- name: UpsertFundNavHistory :one
INSERT INTO fund_nav_history (fund_id, nav_date, nav_price)
VALUES ($1, $2, $3)
ON CONFLICT (fund_id, nav_date) DO UPDATE SET nav_price = EXCLUDED.nav_price
RETURNING *;

-- name: ListFundNavHistory :many
SELECT * FROM fund_nav_history WHERE fund_id = $1 ORDER BY nav_date;

-- name: GetLatestFundNav :one
SELECT * FROM fund_nav_history WHERE fund_id = $1 ORDER BY nav_date DESC LIMIT 1;

-- nisa_contributions -----------------------------------------------------------

-- name: CreateNisaContribution :one
INSERT INTO nisa_contributions (contribution_date, amount, fund_id, contribution_type)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListNisaContributionsByFund :many
SELECT * FROM nisa_contributions WHERE fund_id = $1 ORDER BY contribution_date;

-- name: ListNisaContributions :many
SELECT * FROM nisa_contributions ORDER BY contribution_date;

-- name: GetNisaContributionByFundAndDate :one
SELECT * FROM nisa_contributions WHERE fund_id = $1 AND contribution_date = $2;

-- name: DeleteNisaContributionByFundAndDate :exec
DELETE FROM nisa_contributions WHERE fund_id = $1 AND contribution_date = $2;

-- goals --------------------------------------------------------------------------

-- name: CreateGoal :one
INSERT INTO goals (goal_type, target_value, target_date)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListGoals :many
SELECT * FROM goals ORDER BY id;

-- scenarios ------------------------------------------------------------------------

-- name: CreateScenario :one
INSERT INTO scenarios (params)
VALUES ($1)
RETURNING *;

-- name: GetScenario :one
SELECT * FROM scenarios WHERE id = $1;

-- name: ListScenarios :many
SELECT * FROM scenarios ORDER BY run_at DESC LIMIT $1;

-- forecast_results --------------------------------------------------------------------

-- name: CreateForecastResults :copyfrom
INSERT INTO forecast_results (scenario_id, year, p10, p50, p90)
VALUES ($1, $2, $3, $4, $5);

-- name: ListForecastResultsByScenario :many
SELECT * FROM forecast_results WHERE scenario_id = $1 ORDER BY year;

-- context_summaries -----------------------------------------------------------------------

-- name: CreateContextSummary :one
INSERT INTO context_summaries (scenario_id, summary_json)
VALUES ($1, $2)
RETURNING *;

-- name: GetLatestContextSummary :one
SELECT * FROM context_summaries ORDER BY generated_at DESC LIMIT 1;

-- chat_sessions -----------------------------------------------------------------------------

-- name: CreateChatSession :one
INSERT INTO chat_sessions (context_summary_id)
VALUES ($1)
RETURNING *;

-- name: GetChatSession :one
SELECT * FROM chat_sessions WHERE id = $1;

-- chat_messages -------------------------------------------------------------------------------

-- name: CreateChatMessage :one
INSERT INTO chat_messages (session_id, role, content)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListChatMessagesBySession :many
SELECT * FROM chat_messages WHERE session_id = $1 ORDER BY created_at;

-- bank_accounts -----------------------------------------------------------------------------

-- name: CreateBankAccount :one
INSERT INTO bank_accounts (name)
VALUES ($1)
RETURNING *;

-- name: ListBankAccounts :many
SELECT * FROM bank_accounts ORDER BY id;

-- name: DeleteBankAccount :exec
DELETE FROM bank_accounts WHERE id = $1;

-- bank_account_balances -----------------------------------------------------------------------

-- name: CreateBankAccountBalance :one
INSERT INTO bank_account_balances (monthly_record_id, bank_account_id, amount)
VALUES ($1, $2, $3)
RETURNING *;

-- name: DeleteBankAccountBalancesByMonthlyRecord :exec
DELETE FROM bank_account_balances WHERE monthly_record_id = $1;

-- name: ListBankAccountBalancesByMonthlyRecord :many
SELECT * FROM bank_account_balances WHERE monthly_record_id = $1;

-- name: GetLatestBankAccountBalance :one
SELECT bab.* FROM bank_account_balances bab
JOIN monthly_records mr ON mr.id = bab.monthly_record_id
WHERE bab.bank_account_id = $1
ORDER BY mr.record_month DESC
LIMIT 1;
