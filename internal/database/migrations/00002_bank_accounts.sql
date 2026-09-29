-- +goose Up

-- 口座（個人口座・共用口座など）。NISAファンドと同じ「一度登録し、毎月その口座
-- ごとに残高を入力する」構造（SPEC.md 6章）。monthly_records.bank_balance は
-- 引き続き存在し、口座ごとの残高の合計を保存する（計算エンジンはそちらだけを見る）。
CREATE TABLE bank_accounts (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE TABLE bank_account_balances (
    id SERIAL PRIMARY KEY,
    monthly_record_id INTEGER NOT NULL REFERENCES monthly_records(id) ON DELETE CASCADE,
    bank_account_id INTEGER NOT NULL REFERENCES bank_accounts(id) ON DELETE CASCADE,
    amount INTEGER NOT NULL,
    UNIQUE (monthly_record_id, bank_account_id)
);

CREATE INDEX idx_bank_account_balances_monthly_record_id ON bank_account_balances(monthly_record_id);
CREATE INDEX idx_bank_account_balances_bank_account_id ON bank_account_balances(bank_account_id);

-- +goose Down

DROP TABLE IF EXISTS bank_account_balances CASCADE;
DROP TABLE IF EXISTS bank_accounts CASCADE;
