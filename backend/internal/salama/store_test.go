package salama

import (
	"context"
	"testing"
	"time"

	"fmi/internal/core/cache"
)

// The endpoint promises strikes from the last two hours. When the poller stops
// succeeding (an FMI outage) the retained set is never merged again, so the
// window has to be applied on the way out too: the in-memory fallback has no
// TTL and used to serve the storm of three hours ago as if it were current.
func TestGetDropsStrikesOlderThanTheWindow(t *testing.T) {
	ctx := context.Background()
	lastPoll := time.Now().Add(-3 * time.Hour)
	fresh := []Strike{
		{Latitude: 61, Longitude: 25, Timestamp: lastPoll.Unix()},
		{Latitude: 62, Longitude: 26, Timestamp: time.Now().Add(-10 * time.Minute).Unix()},
	}

	for name, c := range map[string]cache.Cache{
		"cache":    cache.NewMemoryCache(),
		"fallback": nullCache{},
	} {
		s := NewStore(c)
		s.Merge(ctx, fresh, retainWindow, lastPoll)

		got, ok := s.Get(ctx, retainWindow)
		if !ok {
			t.Fatalf("%s: Get reported no data", name)
		}
		if len(got.Strikes) != 1 || got.Strikes[0].Latitude != 62 {
			t.Errorf("%s: got %+v, want only the strike inside the window", name, got.Strikes)
		}
	}
}

// nullCache stands in for Redis being down: writes vanish and reads miss.
type nullCache struct{}

func (nullCache) SetValue(context.Context, string, []byte, time.Duration) error { return nil }
func (nullCache) GetValue(context.Context, string) ([]byte, error)              { return nil, nil }
func (nullCache) Ping(context.Context) error                                    { return nil }
func (nullCache) Close() error                                                  { return nil }
