package api

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

// streamHub fans one per-session ledger poll out to every connected SSE
// client. Before this, N dashboards produced N queries every two seconds for
// the same rows; now the session pays one query per tick and the hub
// serializes each event once. Clients keep their own cursor so a reconnect
// with Last-Event-ID still resumes exactly where it left off.
type streamHub struct {
	ledger *ledger.Ledger

	mu       sync.Mutex
	sessions map[string]*sessionStream
	polls    int64
}

type sessionStream struct {
	hub       *streamHub
	sessionID string
	stop      chan struct{}
	stopOnce  sync.Once

	mu      sync.Mutex
	subs    map[*streamSub]struct{}
	stopped bool
}

type streamSub struct {
	ch     chan streamFrame
	cursor int64
}

type streamFrame struct {
	ID   int64
	Data []byte
}

func newStreamHub(l *ledger.Ledger) *streamHub {
	return &streamHub{ledger: l, sessions: map[string]*sessionStream{}}
}

// pollCount is test/ops evidence that the hub is coalescing clients.
func (h *streamHub) pollCount() int64 { return atomic.LoadInt64(&h.polls) }

func (h *streamHub) subscribe(sessionID string, cursor int64) (*streamSub, func()) {
	h.mu.Lock()
	ss := h.sessions[sessionID]
	if ss != nil {
		ss.mu.Lock()
		stopped := ss.stopped
		ss.mu.Unlock()
		if stopped {
			ss = nil
		}
	}
	sub := &streamSub{ch: make(chan streamFrame, 128), cursor: cursor}
	start := false
	if ss == nil {
		ss = &sessionStream{
			hub:       h,
			sessionID: sessionID,
			stop:      make(chan struct{}),
			subs:      map[*streamSub]struct{}{},
		}
		h.sessions[sessionID] = ss
		start = true
	}
	// Register before starting the poll goroutine: an immediate first poll
	// that saw zero subscribers would shut the stream down and the newly
	// added client would wait forever.
	ss.mu.Lock()
	ss.subs[sub] = struct{}{}
	ss.mu.Unlock()
	if start {
		go ss.run()
	}
	h.mu.Unlock()
	return sub, func() { ss.remove(sub) }
}

func (ss *sessionStream) remove(sub *streamSub) {
	ss.mu.Lock()
	if _, ok := ss.subs[sub]; ok {
		delete(ss.subs, sub)
		close(sub.ch)
	}
	empty := len(ss.subs) == 0
	ss.mu.Unlock()
	if empty {
		ss.shutdown()
	}
}

func (ss *sessionStream) shutdown() {
	ss.stopOnce.Do(func() {
		ss.mu.Lock()
		ss.stopped = true
		ss.mu.Unlock()
		close(ss.stop)
	})
}

func (ss *sessionStream) dropLocked(sub *streamSub) {
	if _, ok := ss.subs[sub]; ok {
		delete(ss.subs, sub)
		close(sub.ch)
	}
}

func (ss *sessionStream) run() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	defer func() {
		ss.hub.mu.Lock()
		if ss.hub.sessions[ss.sessionID] == ss {
			delete(ss.hub.sessions, ss.sessionID)
		}
		ss.hub.mu.Unlock()
	}()
	ss.poll()
	for {
		select {
		case <-ticker.C:
			ss.poll()
		case <-ss.stop:
			return
		}
	}
}

func (ss *sessionStream) poll() {
	ss.mu.Lock()
	if len(ss.subs) == 0 {
		ss.mu.Unlock()
		ss.shutdown()
		return
	}
	var min int64
	first := true
	for sub := range ss.subs {
		if first || sub.cursor < min {
			min, first = sub.cursor, false
		}
	}
	ss.mu.Unlock()

	atomic.AddInt64(&ss.hub.polls, 1)
	events, err := ss.hub.ledger.GetEventsSinceWithGlobalTypes(ss.sessionID,
		[]string{"llm_settings_updated", "llm_provider_verified"}, min, 500)
	if err != nil || len(events) == 0 {
		return
	}
	frames := make([]streamFrame, 0, len(events))
	for _, ev := range events {
		payload, merr := json.Marshal(map[string]any{
			"type":       "telemetry_tick",
			"domain":     "ledger",
			"kind":       ev.EventType,
			"revision":   ev.ID,
			"session_id": ss.sessionID,
			"timestamp":  time.Now().UTC().Format(time.RFC3339),
			"events":     []ledger.EventRecord{ev},
		})
		if merr != nil {
			continue
		}
		frames = append(frames, streamFrame{ID: ev.ID, Data: payload})
	}

	ss.mu.Lock()
	defer ss.mu.Unlock()
	for sub := range ss.subs {
		for _, f := range frames {
			if f.ID <= sub.cursor {
				continue
			}
			select {
			case sub.ch <- f:
				sub.cursor = f.ID
			default:
				// A client that cannot keep up reconnects with Last-Event-ID
				// and replays from its last acknowledged id; blocking the hub
				// on one slow reader would stall every other dashboard.
				ss.dropLocked(sub)
			}
		}
	}
}

func (s *Server) getStreamHub() *streamHub {
	if s == nil {
		return nil
	}
	s.streamHubMu.Lock()
	defer s.streamHubMu.Unlock()
	if s.streamHub == nil && s.ledger != nil {
		s.streamHub = newStreamHub(s.ledger)
	}
	return s.streamHub
}
