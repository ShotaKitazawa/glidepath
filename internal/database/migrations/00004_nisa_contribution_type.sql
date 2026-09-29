-- +goose Up

-- recurring（毎月の定期積立。口数計算にも将来の年間拠出ペース見積もりにも使う） /
-- spot（不定期だが実際に入金した単発購入。口数計算には使うが、拠出ペースの見積もり
-- には含めない） / snapshot（初期保有額記録専用。その日時点の保有評価額そのものの
-- 申告であり入金ではないため、口数計算ではこれ以前の全ての行を置き換える
-- — calc.ActiveContributions参照）。SPEC.md 4.1/4.3参照。
ALTER TABLE nisa_contributions ADD COLUMN contribution_type TEXT NOT NULL DEFAULT 'recurring';

-- +goose Down

ALTER TABLE nisa_contributions DROP COLUMN IF EXISTS contribution_type;
