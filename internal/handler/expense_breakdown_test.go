package handler

import (
	"reflect"
	"testing"
)

func TestParseExpenseItems(t *testing.T) {
	tests := []struct {
		name    string
		labels  []string
		amounts []string
		want    []expenseBreakdownItem
	}{
		{
			name:    "labeled",
			labels:  []string{"食費", "日用品", "交通費"},
			amounts: []string{"15", "32", "8"},
			want: []expenseBreakdownItem{
				{Label: "食費", Amount: 15},
				{Label: "日用品", Amount: 32},
				{Label: "交通費", Amount: 8},
			},
		},
		{
			name:    "blank label falls back to expenseTotalCategory",
			labels:  []string{"", "", ""},
			amounts: []string{"15", "32", "8"},
			want: []expenseBreakdownItem{
				{Label: expenseTotalCategory, Amount: 15},
				{Label: expenseTotalCategory, Amount: 32},
				{Label: expenseTotalCategory, Amount: 8},
			},
		},
		{
			name:    "mixed labeled and blank",
			labels:  []string{"食費", "", "交通費"},
			amounts: []string{"15", "32", "8"},
			want: []expenseBreakdownItem{
				{Label: "食費", Amount: 15},
				{Label: expenseTotalCategory, Amount: 32},
				{Label: "交通費", Amount: 8},
			},
		},
		{
			name:    "empty trailing row from + 項目を追加 skipped",
			labels:  []string{"食費", ""},
			amounts: []string{"15", ""},
			want:    []expenseBreakdownItem{{Label: "食費", Amount: 15}},
		},
		{
			name:    "malformed amount skipped",
			labels:  []string{"食費", "日用品"},
			amounts: []string{"abc", "32"},
			want:    []expenseBreakdownItem{{Label: "日用品", Amount: 32}},
		},
		{
			name:    "whitespace trimmed",
			labels:  []string{" 食費 "},
			amounts: []string{" 15 "},
			want:    []expenseBreakdownItem{{Label: "食費", Amount: 15}},
		},
		{
			name:    "no rows",
			labels:  nil,
			amounts: nil,
			want:    []expenseBreakdownItem{},
		},
		{
			name:    "zero and negative amounts skipped",
			labels:  []string{"食費", "日用品", "交通費"},
			amounts: []string{"0", "-5", "8"},
			want:    []expenseBreakdownItem{{Label: "交通費", Amount: 8}},
		},
		{
			name:    "fewer labels than amounts falls back for the extras",
			labels:  []string{"食費"},
			amounts: []string{"15", "32"},
			want: []expenseBreakdownItem{
				{Label: "食費", Amount: 15},
				{Label: expenseTotalCategory, Amount: 32},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseExpenseItems(tt.labels, tt.amounts)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseExpenseItems(%v, %v) = %#v, want %#v", tt.labels, tt.amounts, got, tt.want)
			}
		})
	}
}
