//go:build tools

// Package tools pins go-run-style tool dependencies in go.mod/go.sum.
package tools

import (
	_ "github.com/pressly/goose/v3/cmd/goose"
	_ "github.com/sqlc-dev/sqlc/cmd/sqlc"
)
