package settings

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/cardinal-grammeter/internal/engine"
)

// Saver writes configurations to a Store from one background goroutine, so a
// slow or failing API server never delays PUT /v1/traffic. Only the newest
// configuration matters: one submitted while another is being written replaces
// any older one still waiting, and a failed write is retried with backoff until
// it lands or a newer configuration takes its place.
type Saver struct {
	store Store
	log   *slog.Logger

	// Timeout bounds one write; MinBackoff and MaxBackoff bound the wait between
	// failed ones. Set them before Run.
	Timeout    time.Duration
	MinBackoff time.Duration
	MaxBackoff time.Duration

	mu      sync.Mutex
	pending *engine.Config // newest configuration not yet written, nil when none
	seq     uint64         // bumped by every Submit

	wake chan struct{}
	stop chan struct{}
	done chan struct{}
	once sync.Once
}

// NewSaver builds a Saver. Call Run to start it and Close to stop it.
func NewSaver(store Store, log *slog.Logger) *Saver {
	if log == nil {
		log = slog.Default()
	}
	return &Saver{
		store:      store,
		log:        log,
		Timeout:    5 * time.Second,
		MinBackoff: time.Second,
		MaxBackoff: 30 * time.Second,
		wake:       make(chan struct{}, 1),
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
	}
}

// Submit queues c to be written and returns at once. It is safe to call with
// the engine's lock held (engine.Options.OnChange).
func (s *Saver) Submit(c engine.Config) {
	s.mu.Lock()
	s.pending = &c
	s.seq++
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Run writes queued configurations until Close is called.
func (s *Saver) Run() {
	defer close(s.done)
	backoff := time.Duration(0)
	for {
		if backoff == 0 {
			select {
			case <-s.stop:
				return
			case <-s.wake:
			}
		}
		cfg, seq, ok := s.next()
		if !ok {
			backoff = 0
			continue
		}
		if err := s.write(cfg, seq); err != nil {
			backoff = s.nextBackoff(backoff)
			s.log.Warn("could not save the traffic settings; will retry",
				"error", err.Error(), "retryIn", backoff.String())
			select {
			case <-s.stop:
				return
			case <-s.wake: // a newer configuration: try that one straight away
			case <-time.After(backoff):
			}
			continue
		}
		backoff = 0
		// Something newer may have arrived while this one was being written.
		s.mu.Lock()
		more := s.pending != nil
		s.mu.Unlock()
		if more {
			select {
			case s.wake <- struct{}{}:
			default:
			}
		}
	}
}

// Close stops Run, then writes whatever is still pending once, within ctx. It
// is for shutdown: the last change a pod accepted must not be lost with it.
func (s *Saver) Close(ctx context.Context) error {
	s.once.Do(func() { close(s.stop) })
	select {
	case <-s.done:
	case <-ctx.Done():
		return ctx.Err()
	}
	cfg, seq, ok := s.next()
	if !ok {
		return nil
	}
	if err := s.writeCtx(ctx, cfg, seq); err != nil {
		s.log.Warn("could not save the traffic settings before shutting down", "error", err.Error())
		return err
	}
	return nil
}

// next returns the newest pending configuration and its sequence number.
func (s *Saver) next() (engine.Config, uint64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending == nil {
		return engine.Config{}, 0, false
	}
	return *s.pending, s.seq, true
}

func (s *Saver) write(cfg engine.Config, seq uint64) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.Timeout)
	defer cancel()
	return s.writeCtx(ctx, cfg, seq)
}

func (s *Saver) writeCtx(ctx context.Context, cfg engine.Config, seq uint64) error {
	data, err := Encode(cfg)
	if err != nil {
		return err
	}
	if err := s.store.Save(ctx, data); err != nil {
		return err
	}
	s.mu.Lock()
	if s.seq == seq { // nothing newer was submitted meanwhile
		s.pending = nil
	}
	s.mu.Unlock()
	s.log.Info("saved the traffic settings", "running", cfg.Running, "rps", cfg.RPS,
		"pattern", string(cfg.Pattern))
	return nil
}

func (s *Saver) nextBackoff(cur time.Duration) time.Duration {
	if cur == 0 {
		return s.MinBackoff
	}
	cur *= 2
	if cur > s.MaxBackoff {
		cur = s.MaxBackoff
	}
	return cur
}
