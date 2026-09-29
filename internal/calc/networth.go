package calc

// NetWorth computes 純資産 (SPEC.md 4.2: net_worth = bank_balance + NISA評価額).
func NetWorth(bankBalanceYen, nisaValuationYen int) int {
	return bankBalanceYen + nisaValuationYen
}
