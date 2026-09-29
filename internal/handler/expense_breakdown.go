package handler

import (
	"strconv"
	"strings"
)

// expenseTotalCategory is the fallback expense_categories row written when
// the user types a total directly with no labeled items.
const expenseTotalCategory = "支出合計"

// expenseBreakdownItem is one labeled expense line item from the monthly
// form. Amount is in 千円 (matching the surrounding form), not yet
// converted to raw yen.
type expenseBreakdownItem struct {
	Label  string
	Amount int
}

// parseExpenseItems pairs up the monthly form's repeated expense_label /
// expense_amount fields (one dynamically-added row each — see
// inventory.html's "+ 項目を追加" button) into items. A "+"-delimited
// single-field format was tried first and dropped as too easy to mistype
// and impossible to correct one entry of without retyping the whole
// string.
//
// The label exists so a user can recall next month what they logged this
// month (renderInventory's PreviousExpenseItems) — it's stored as
// expense_categories.category, same table the app once used for a fixed
// category list before that was dropped (SPEC.md 6章 note); labels here
// are freeform and user-chosen, not that fixed list. A blank label falls
// back to expenseTotalCategory. Rows with a non-positive or unparseable
// amount are silently skipped (covers a still-empty trailing row from
// "+ 項目を追加").
func parseExpenseItems(labels, amounts []string) []expenseBreakdownItem {
	items := make([]expenseBreakdownItem, 0, len(amounts))
	for i, amountText := range amounts {
		amount, err := strconv.Atoi(strings.TrimSpace(amountText))
		if err != nil || amount <= 0 {
			continue
		}

		label := expenseTotalCategory
		if i < len(labels) {
			if l := strings.TrimSpace(labels[i]); l != "" {
				label = l
			}
		}

		items = append(items, expenseBreakdownItem{Label: label, Amount: amount})
	}
	return items
}
