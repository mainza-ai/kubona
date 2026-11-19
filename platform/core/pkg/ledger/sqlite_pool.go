package ledger

import (
	"os"
	"strconv"
	"strings"
)

// sqliteMaxOpenConns bounds the pool. SQLite serializes writers but permits
// concurrent WAL readers; eight is enough for the API's read mix without
// letting an unbounded database/sql pool multiply the per-connection page
// cache. Operators can lower it on small hosts.
func sqliteMaxOpenConns() int {
	const def = 8
	raw := strings.TrimSpace(os.Getenv("KUBONA_SQLITE_MAX_OPEN_CONNS"))
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return def
	}
	if n > 32 {
		n = 32
	}
	return n
}
