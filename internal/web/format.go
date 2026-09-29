package web

import (
	"html/template"
	"strconv"
	"strings"
)

// funcMap is registered on every page template (see Render). Input fields
// stay in 千円 (thousand yen) — short numbers are easier to type — but
// read-only display was found confusing left in the same unit ("流石に見
// にくい"), so these convert to actual yen with thousands separators for
// anything the user only reads, not types into.
var funcMap = template.FuncMap{
	"yen":    formatThousandYen,
	"yenRaw": formatRawYen,
}

// formatThousandYen renders a value stored in thousand-yen units (as most
// of this app's view structs are, per internal/handler/money.go) as an
// actual-yen string with thousands separators, e.g. 2114 -> "2,114,000".
func formatThousandYen(thousandYen int32) string {
	return commaYen(int64(thousandYen) * 1000)
}

// formatRawYen renders a value already in yen (e.g. a fund's NAV price,
// which money.go deliberately keeps unconverted) with thousands
// separators, e.g. 43966 -> "43,966".
func formatRawYen(rawYen int32) string {
	return commaYen(int64(rawYen))
}

func commaYen(yen int64) string {
	neg := yen < 0
	if neg {
		yen = -yen
	}
	s := strconv.FormatInt(yen, 10)
	if len(s) <= 3 {
		if neg {
			return "-" + s
		}
		return s
	}

	var b strings.Builder
	first := len(s) % 3
	if first == 0 {
		first = 3
	}
	b.WriteString(s[:first])
	for i := first; i < len(s); i += 3 {
		b.WriteByte(',')
		b.WriteString(s[i : i+3])
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}
