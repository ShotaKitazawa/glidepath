package mufg

import (
	"context"
	"errors"
	"time"
)

// Fetcher is the subset of *Client that SyncQuotes needs; letting callers
// pass a fake keeps tests from hitting the network.
type Fetcher interface {
	FetchByDate(ctx context.Context, fundCode string, d time.Time) (Quote, error)
}

// SyncQuotes walks every calendar day in [from, to] (inclusive) calling
// fetcher.FetchByDate, and returns the days that had data. ErrNoData days
// (weekends, market holidays, before fund inception) are silently skipped;
// any other error aborts and is returned. delay is slept between requests
// to be a polite API citizen (pass 0 in tests).
func SyncQuotes(ctx context.Context, fetcher Fetcher, fundCode string, from, to time.Time, delay time.Duration) ([]Quote, error) {
	var quotes []Quote
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		q, err := fetcher.FetchByDate(ctx, fundCode, d)
		if err != nil {
			if errors.Is(err, ErrNoData) {
				continue
			}
			return nil, err
		}
		quotes = append(quotes, q)

		if delay > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}
	}
	return quotes, nil
}
