// Command backfill-nav does a one-time historical 基準価額 backfill for a
// fund already registered via the web UI (which only syncs a bounded
// recent window per click — see internal/handler/funds.go). Run it once
// after adding a fund, from its inception date:
//
//	DATABASE_URL=... go run ./cmd/backfill-nav --fund-id 1 --from 2018-07-03
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ShotaKitazawa/glidepath/internal/database/sqlcgen"
	"github.com/ShotaKitazawa/glidepath/internal/mufg"
)

func main() {
	fundID := flag.Int("fund-id", 0, "funds.id of an already-registered fund (required)")
	fromStr := flag.String("from", "", "backfill start date, YYYY-MM-DD (required; typically the fund's inception date)")
	toStr := flag.String("to", "", "backfill end date, YYYY-MM-DD (default: yesterday)")
	delay := flag.Duration("delay", 300*time.Millisecond, "delay between API requests")
	flag.Parse()

	if err := run(*fundID, *fromStr, *toStr, *delay); err != nil {
		slog.Error("backfill failed", "error", err)
		os.Exit(1)
	}
}

func run(fundID int, fromStr, toStr string, delay time.Duration) error {
	if fundID == 0 {
		return fmt.Errorf("--fund-id is required")
	}
	if fromStr == "" {
		return fmt.Errorf("--from is required")
	}
	from, err := time.Parse("2006-01-02", fromStr)
	if err != nil {
		return fmt.Errorf("invalid --from: %w", err)
	}
	to := time.Now().AddDate(0, 0, -1)
	if toStr != "" {
		to, err = time.Parse("2006-01-02", toStr)
		if err != nil {
			return fmt.Errorf("invalid --to: %w", err)
		}
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL must be set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()
	q := sqlcgen.New(pool)

	fund, err := q.GetFund(ctx, int32(fundID))
	if err != nil {
		return fmt.Errorf("looking up fund %d: %w", fundID, err)
	}
	if !fund.IsinOrCode.Valid || fund.IsinOrCode.String == "" {
		return fmt.Errorf("fund %d has no MUFG fund code set (funds.isin_or_code)", fundID)
	}

	client := mufg.NewClient()
	quotes, err := mufg.SyncQuotes(ctx, client, fund.IsinOrCode.String, from, to, delay)
	if err != nil {
		return fmt.Errorf("fetching NAV history: %w", err)
	}

	for _, quote := range quotes {
		if _, err := q.UpsertFundNavHistory(ctx, sqlcgen.UpsertFundNavHistoryParams{
			FundID:   fund.ID,
			NavDate:  pgtype.Date{Time: quote.Date, Valid: true},
			NavPrice: int32(quote.NAVYen),
		}); err != nil {
			return fmt.Errorf("saving quote for %s: %w", quote.Date.Format("2006-01-02"), err)
		}
	}

	slog.Info("backfill complete", "fund", fund.Name, "quotes", len(quotes), "from", from.Format("2006-01-02"), "to", to.Format("2006-01-02"))
	return nil
}
