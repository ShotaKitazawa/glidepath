-- +goose Up

-- 自前の基準価額データを持たないファンド（例: 運用会社に公開APIが無い）の
-- 値動きを、別の登録済みファンドの基準価額履歴で代用するための参照。SPEC.md
-- 4.1参照。設定すると、評価額・モンテカルロのリターンサンプリングは自分の
-- fund_nav_historyではなく参照先のものを使う。自分の基準価額同期
-- （isin_or_code / nav_source_url）とは併用しない想定。
ALTER TABLE funds ADD COLUMN nav_proxy_fund_id INTEGER REFERENCES funds(id) ON DELETE SET NULL;

-- +goose Down

ALTER TABLE funds DROP COLUMN nav_proxy_fund_id;
