package board

// Helpers the mapping tests need, kept beside them so the test file reads as one
// argument. `pgxErrNoRows` returns the real sentinel; `wrappedNoRows` returns it
// the way a query helper does, wrapped in a %w chain.

import (
	"fmt"

	"github.com/jackc/pgx/v5"
)

func pgxErrNoRows() error { return pgx.ErrNoRows }

func wrappedNoRows() error { return fmt.Errorf("get project: %w", pgx.ErrNoRows) }
