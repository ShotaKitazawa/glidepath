package handler

import "testing"

func TestExtractFundCodeFromURL(t *testing.T) {
	cases := []struct {
		name     string
		url      string
		wantCode string
		wantOK   bool
	}{
		{"typical MUFG fund page URL", "https://www.am.mufg.jp/fund/253266.html", "253266", true},
		{"http scheme", "http://www.am.mufg.jp/fund/253266.html", "253266", true},
		{"trailing slash, no extension", "https://www.am.mufg.jp/fund/253266/", "253266", true},
		{"query string", "https://www.am.mufg.jp/fund/253266.html?utm_source=x", "253266", true},
		{"empty", "", "", false},
		{"unrelated URL", "https://example.com/about", "", false},
		{"no numeric code in path", "https://www.am.mufg.jp/fund/", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, ok := extractFundCodeFromURL(tc.url)
			if ok != tc.wantOK || code != tc.wantCode {
				t.Errorf("extractFundCodeFromURL(%q) = (%q, %v), want (%q, %v)", tc.url, code, ok, tc.wantCode, tc.wantOK)
			}
		})
	}
}
