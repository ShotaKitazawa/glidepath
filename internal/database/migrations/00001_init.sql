-- +goose Up

-- 実績（月次棚卸しで入力）
CREATE TABLE monthly_records (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    record_month DATE NOT NULL UNIQUE,
    income_monthly INTEGER NOT NULL,      -- 年次入力を月按分
    bank_balance INTEGER NOT NULL,        -- 個人口座+共用口座 合算残高（実額入力）
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE expense_categories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    monthly_record_id INTEGER NOT NULL REFERENCES monthly_records(id) ON DELETE CASCADE,
    category TEXT NOT NULL,               -- 住宅ローン/食費/教育費 等
    amount INTEGER NOT NULL
);

CREATE INDEX idx_expense_categories_monthly_record_id ON expense_categories(monthly_record_id);

-- 家族構成・教育費
CREATE TABLE family_members (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    relation TEXT NOT NULL,
    birth_month DATE NOT NULL
);

CREATE TABLE expense_forecasts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    family_member_id INTEGER NOT NULL REFERENCES family_members(id) ON DELETE CASCADE,
    stage TEXT NOT NULL,                  -- 幼稚園/小学校/中学校/高校/大学
    track TEXT NOT NULL,                  -- 公立/私立
    annual_cost INTEGER NOT NULL,
    start_age INTEGER NOT NULL,
    end_age INTEGER NOT NULL,
    is_override BOOLEAN NOT NULL DEFAULT false -- 実額で上書きしたか
);

CREATE INDEX idx_expense_forecasts_family_member_id ON expense_forecasts(family_member_id);

-- 周期的大型出費
CREATE TABLE big_purchases (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    base_amount INTEGER NOT NULL,
    base_date DATE NOT NULL,
    cycle_years INTEGER NOT NULL,
    category_growth_rate REAL,            -- カテゴリ固有の値上がり率（デフォルトは統計値、上書き可）
    trade_in_value INTEGER NOT NULL DEFAULT 0,
    recurring BOOLEAN NOT NULL DEFAULT true,
    financing_mode TEXT NOT NULL DEFAULT 'cash' -- cash / loan_if_favorable（判定ロジックは将来拡張）
);

-- NISA（口数ベース、評価額は都度算出。手入力は入金額のみ）
CREATE TABLE funds (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    isin_or_code TEXT,
    nav_source_url TEXT                   -- 運用会社公式サイトのCSV取得元
);

CREATE TABLE fund_nav_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    fund_id INTEGER NOT NULL REFERENCES funds(id) ON DELETE CASCADE,
    nav_date DATE NOT NULL,
    nav_price INTEGER NOT NULL,           -- 基準価額（1万口あたり）
    UNIQUE (fund_id, nav_date)
);

CREATE INDEX idx_fund_nav_history_fund_id_date ON fund_nav_history(fund_id, nav_date);

CREATE TABLE nisa_contributions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    contribution_date DATE NOT NULL,
    amount INTEGER NOT NULL,
    fund_id INTEGER NOT NULL REFERENCES funds(id) ON DELETE CASCADE
);

CREATE INDEX idx_nisa_contributions_fund_id ON nisa_contributions(fund_id);

-- 目標・シミュレーション
CREATE TABLE goals (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    goal_type TEXT NOT NULL,
    target_value REAL NOT NULL,
    target_date DATE
);

CREATE TABLE scenarios (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    params TEXT NOT NULL
);

CREATE TABLE forecast_results (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    scenario_id INTEGER NOT NULL REFERENCES scenarios(id) ON DELETE CASCADE,
    year INTEGER NOT NULL,
    p10 INTEGER NOT NULL,
    p50 INTEGER NOT NULL,
    p90 INTEGER NOT NULL
);

CREATE INDEX idx_forecast_results_scenario_id ON forecast_results(scenario_id);

-- 対話
CREATE TABLE context_summaries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    generated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    scenario_id INTEGER REFERENCES scenarios(id) ON DELETE SET NULL,
    summary_json TEXT NOT NULL
    -- 例: {
    --   "net_worth_now": 12000000,
    --   "net_worth_2047_p10_p50_p90": [45000000, 68000000, 95000000],
    --   "savings_rate_change": "+2.3pt (前回比)",
    --   "alerts": ["生活防衛資金は目標の80%"]
    -- }
);

CREATE TABLE chat_sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    started_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    context_summary_id INTEGER REFERENCES context_summaries(id) ON DELETE SET NULL
);

CREATE TABLE chat_messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
    role TEXT NOT NULL,                   -- 'user' | 'assistant'
    content TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_chat_messages_session_id ON chat_messages(session_id);

-- +goose Down

DROP TABLE IF EXISTS chat_messages;
DROP TABLE IF EXISTS chat_sessions;
DROP TABLE IF EXISTS context_summaries;
DROP TABLE IF EXISTS forecast_results;
DROP TABLE IF EXISTS scenarios;
DROP TABLE IF EXISTS goals;
DROP TABLE IF EXISTS nisa_contributions;
DROP TABLE IF EXISTS fund_nav_history;
DROP TABLE IF EXISTS funds;
DROP TABLE IF EXISTS big_purchases;
DROP TABLE IF EXISTS expense_forecasts;
DROP TABLE IF EXISTS family_members;
DROP TABLE IF EXISTS expense_categories;
DROP TABLE IF EXISTS monthly_records;
