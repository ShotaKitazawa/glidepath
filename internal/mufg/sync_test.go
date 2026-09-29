package mufg

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeFetcher struct {
	byDate map[string]Quote // "2006-01-02" -> Quote; absent = ErrNoData
	calls  []time.Time
}

func (f *fakeFetcher) FetchByDate(ctx context.Context, fundCode string, d time.Time) (Quote, error) {
	f.calls = append(f.calls, d)
	q, ok := f.byDate[d.Format("2006-01-02")]
	if !ok {
		return Quote{}, ErrNoData
	}
	return q, nil
}

func TestSyncQuotes_SkipsNoDataDays(t *testing.T) {
	fake := &fakeFetcher{byDate: map[string]Quote{
		"2024-01-01": {Date: date(t, "2024-01-01"), NAVYen: 100},
		// 2024-01-02, 2024-01-03: no data (weekend/holiday)
		"2024-01-04": {Date: date(t, "2024-01-04"), NAVYen: 103},
	}}

	quotes, err := SyncQuotes(context.Background(), fake, "253266", date(t, "2024-01-01"), date(t, "2024-01-04"), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(quotes) != 2 {
		t.Fatalf("got %d quotes, want 2: %v", len(quotes), quotes)
	}
	if quotes[0].NAVYen != 100 || quotes[1].NAVYen != 103 {
		t.Errorf("unexpected quotes: %v", quotes)
	}
	if len(fake.calls) != 4 {
		t.Errorf("expected 4 calls (one per day in range), got %d", len(fake.calls))
	}
}

func TestSyncQuotes_PropagatesHardErrors(t *testing.T) {
	fetcher := fetcherFunc(func(ctx context.Context, fundCode string, d time.Time) (Quote, error) {
		return Quote{}, errors.New("boom")
	})

	_, err := SyncQuotes(context.Background(), fetcher, "253266", date(t, "2024-01-01"), date(t, "2024-01-01"), 0)
	if err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestSyncQuotes_FromAfterTo(t *testing.T) {
	fake := &fakeFetcher{}
	quotes, err := SyncQuotes(context.Background(), fake, "253266", date(t, "2024-01-05"), date(t, "2024-01-01"), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(quotes) != 0 {
		t.Errorf("expected no quotes, got %v", quotes)
	}
	if len(fake.calls) != 0 {
		t.Errorf("expected no calls, got %d", len(fake.calls))
	}
}

type fetcherFunc func(ctx context.Context, fundCode string, d time.Time) (Quote, error)

func (f fetcherFunc) FetchByDate(ctx context.Context, fundCode string, d time.Time) (Quote, error) {
	return f(ctx, fundCode, d)
}
