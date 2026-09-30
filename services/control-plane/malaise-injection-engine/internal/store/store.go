// Package store keeps malaise records in Valkey, under the malaise:* keys this
// service owns.
//
// Three keys carry the whole state:
//
//	malaise:record:<id>  the JSON record, with a generous TTL so recent history
//	                     survives long enough to be looked at
//	malaise:active       a sorted set of ids scored by expiry, so the reconcile
//	                     loop can ask "what is due?" in one round trip
//	malaise:history      a sorted set of ids scored by creation, trimmed, so the
//	                     console can list what has happened lately
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/malaise-injection-engine/internal/malaise"
)

// Key prefixes and names.
const (
	recordPrefix = "malaise:record:"
	activeKey    = "malaise:active"
	historyKey   = "malaise:history"
)

// HistoryTTL is how long a finished malaise stays readable.
const HistoryTTL = 2 * time.Hour

// HistoryLimit caps how many records the history index keeps.
const HistoryLimit = 500

// ErrNotFound means no record exists under that id.
var ErrNotFound = errors.New("malaise not found")

// Store is the Valkey-backed malaise store.
type Store struct {
	rdb redis.UniversalClient
	now func() time.Time
}

// Config configures a Store.
type Config struct {
	// URL is VALKEY_URL, for example redis://default:pw@valkey:6379/0.
	URL string
	// Client overrides URL (tests supply a miniredis-backed client).
	Client redis.UniversalClient
	// Now may be nil, meaning time.Now.
	Now func() time.Time
}

// New builds a Store.
func New(cfg Config) (*Store, error) {
	client := cfg.Client
	if client == nil {
		if cfg.URL == "" {
			return nil, errors.New("store: VALKEY_URL is required")
		}
		opts, err := redis.ParseURL(cfg.URL)
		if err != nil {
			return nil, fmt.Errorf("store: parse VALKEY_URL: %w", err)
		}
		client = redis.NewClient(opts)
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Store{rdb: client, now: cfg.Now}, nil
}

// Close releases the connection pool.
func (s *Store) Close() error { return s.rdb.Close() }

// Ping checks that Valkey answers, for the readiness probe.
func (s *Store) Ping(ctx context.Context) error {
	return s.rdb.Ping(ctx).Err()
}

// recordTTL keeps a record readable until well after it expires.
func (s *Store) recordTTL(m malaise.Malaise) time.Duration {
	ttl := m.ExpiresAt.Sub(s.now()) + HistoryTTL
	if ttl < HistoryTTL {
		ttl = HistoryTTL
	}
	return ttl
}

// Put writes a record and updates the indexes. An active record joins
// malaise:active; a terminal one leaves it.
func (s *Store) Put(ctx context.Context, m malaise.Malaise) error {
	encoded, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("encode malaise: %w", err)
	}

	pipe := s.rdb.TxPipeline()
	pipe.Set(ctx, recordPrefix+m.ID, encoded, s.recordTTL(m))
	pipe.ZAdd(ctx, historyKey, redis.Z{
		Score:  float64(m.CreatedAt.UnixMilli()),
		Member: m.ID,
	})
	// Keep the newest HistoryLimit entries: rank 0 is the oldest.
	pipe.ZRemRangeByRank(ctx, historyKey, 0, -(HistoryLimit + 1))
	if m.State == malaise.StateActive {
		pipe.ZAdd(ctx, activeKey, redis.Z{
			Score:  float64(m.ExpiresAt.UnixMilli()),
			Member: m.ID,
		})
	} else {
		pipe.ZRem(ctx, activeKey, m.ID)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("write malaise %s: %w", m.ID, err)
	}
	return nil
}

// Get reads one record.
func (s *Store) Get(ctx context.Context, id string) (malaise.Malaise, error) {
	raw, err := s.rdb.Get(ctx, recordPrefix+id).Bytes()
	if errors.Is(err, redis.Nil) {
		return malaise.Malaise{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if err != nil {
		return malaise.Malaise{}, fmt.Errorf("read malaise %s: %w", id, err)
	}
	var m malaise.Malaise
	if err := json.Unmarshal(raw, &m); err != nil {
		return malaise.Malaise{}, fmt.Errorf("decode malaise %s: %w", id, err)
	}
	return m, nil
}

// Active returns every record still in the active index, soonest expiry first,
// which is the order the console wants for its countdowns. Records whose id is
// indexed but whose body has gone are pruned as a side effect.
func (s *Store) Active(ctx context.Context) ([]malaise.Malaise, error) {
	ids, err := s.rdb.ZRange(ctx, activeKey, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("read active index: %w", err)
	}
	return s.load(ctx, ids, activeKey)
}

// Recent returns the most recently created records, newest first, active or not.
func (s *Store) Recent(ctx context.Context, limit int) ([]malaise.Malaise, error) {
	if limit <= 0 || limit > HistoryLimit {
		limit = HistoryLimit
	}
	ids, err := s.rdb.ZRevRange(ctx, historyKey, 0, int64(limit-1)).Result()
	if err != nil {
		return nil, fmt.Errorf("read history index: %w", err)
	}
	return s.load(ctx, ids, historyKey)
}

// DueBefore returns the ids of active malaises whose expiry is at or before t.
func (s *Store) DueBefore(ctx context.Context, t time.Time) ([]string, error) {
	ids, err := s.rdb.ZRangeByScore(ctx, activeKey, &redis.ZRangeBy{
		Min: "-inf",
		Max: fmt.Sprintf("%d", t.UnixMilli()),
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("read due malaises: %w", err)
	}
	return ids, nil
}

// Deactivate removes an id from the active index without touching the record.
func (s *Store) Deactivate(ctx context.Context, id string) error {
	if err := s.rdb.ZRem(ctx, activeKey, id).Err(); err != nil {
		return fmt.Errorf("deactivate %s: %w", id, err)
	}
	return nil
}

// load fetches records for ids, newest first, pruning index entries whose
// record has expired out from under them.
func (s *Store) load(ctx context.Context, ids []string, indexKey string) ([]malaise.Malaise, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = recordPrefix + id
	}
	values, err := s.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("read malaise records: %w", err)
	}

	out := make([]malaise.Malaise, 0, len(values))
	var missing []string
	for i, v := range values {
		raw, ok := asBytes(v)
		if !ok {
			missing = append(missing, ids[i])
			continue
		}
		var m malaise.Malaise
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("decode malaise %s: %w", ids[i], err)
		}
		out = append(out, m)
	}
	if len(missing) > 0 {
		members := make([]any, len(missing))
		for i, id := range missing {
			members[i] = id
		}
		if err := s.rdb.ZRem(ctx, indexKey, members...).Err(); err != nil {
			return out, fmt.Errorf("prune index %s: %w", indexKey, err)
		}
	}
	return out, nil
}

func asBytes(v any) ([]byte, bool) {
	switch typed := v.(type) {
	case string:
		return []byte(typed), true
	case []byte:
		return typed, true
	default:
		return nil, false
	}
}
