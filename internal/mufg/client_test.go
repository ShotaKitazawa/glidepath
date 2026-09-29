package mufg

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchByDate_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/fund_information_date/fund_cd/253266/base_date/20240104"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"result": {"status": 200, "errcd": null, "errmsg": null},
			"errors": {"count": 0},
			"datasets": [{"fund_cd":"253266","base_date":"20240104","nav":18374}]
		}`))
	}))
	defer srv.Close()

	client := NewClient(WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	quote, err := client.FetchByDate(context.Background(), "253266", date(t, "2024-01-04"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if quote.NAVYen != 18374 {
		t.Errorf("NAVYen = %d, want 18374", quote.NAVYen)
	}
	if !quote.Date.Equal(date(t, "2024-01-04")) {
		t.Errorf("Date = %v, want 2024-01-04", quote.Date)
	}
}

func TestFetchByDate_NoData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"result": {"status": 404, "errcd": "BIZ00018", "errmsg": "業務エラーが発生しました。"},
			"errors": {"count": 1},
			"datasets": null
		}`))
	}))
	defer srv.Close()

	client := NewClient(WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	_, err := client.FetchByDate(context.Background(), "253266", date(t, "2024-01-02"))
	if err != ErrNoData {
		t.Fatalf("err = %v, want ErrNoData", err)
	}
}

func TestFetchByDate_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"result": {"status": 500, "errmsg": "Internal Server Error"}, "datasets": null}`))
	}))
	defer srv.Close()

	client := NewClient(WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	_, err := client.FetchByDate(context.Background(), "253266", date(t, "2024-01-04"))
	if err == nil {
		t.Fatal("expected an error")
	}
}

func date(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("bad test date %q: %v", s, err)
	}
	return d
}
