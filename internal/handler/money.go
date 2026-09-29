package handler

// All monetary amounts are stored in the DB as raw yen (SPEC.md's schema).
// Every screen displays and accepts amounts in 千円 (thousand yen) instead
// — the conversion happens only at this UI boundary, never in calc or the
// DB layer. The one exception is a fund's 基準価額 (NAV price per 10,000
// units): it's a per-unit price index, not a spendable amount, so it stays
// in raw yen.

// toThousandYen converts a raw yen amount for display. It takes int (not
// int32) because a Monte Carlo forecast's raw-yen net worth can exceed
// int32's range after decades of compounding — narrowing must happen after
// dividing by 1000, not before.
func toThousandYen(yen int) int32 {
	return int32(yen / 1000)
}

// fromThousandYen converts a 千円 form value back to raw yen for storage.
func fromThousandYen(thousandYen int) int32 {
	return int32(thousandYen * 1000)
}
