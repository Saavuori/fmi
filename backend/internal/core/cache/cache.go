package cache

import (
	"context"
	"time"
)

// Cache is the shared live-data cache: every mode here is poll-style, so the
// contract is a single generic TTL key/value store. Pollers write whole
// snapshot blobs under mode-prefixed keys (e.g. "fmi:tutka:index",
// "fmi:salama:strikes") and handlers read them back.
//
// This is a cache and not a store. The app must keep working with Redis down —
// each mode's Store mirrors what it writes in-process — and the only persistent
// data in the system is the radar frame archive on disk.
type Cache interface {
	// SetValue stores payload under key with a TTL. GetValue returns nil with
	// no error when the key is missing or expired.
	SetValue(ctx context.Context, key string, payload []byte, ttl time.Duration) error
	GetValue(ctx context.Context, key string) ([]byte, error)

	Ping(ctx context.Context) error
	Close() error
}
