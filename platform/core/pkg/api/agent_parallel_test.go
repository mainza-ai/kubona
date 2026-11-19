package api

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/llm"
	"github.com/mainza-ai/kubona/core/pkg/tools"
)

func TestReadOnlyToolCallsRunConcurrently(t *testing.T) {
	reg := tools.NewRegistry()
	var active, peak int64
	mk := func(name string) tools.Definition {
		return tools.Definition{
			Name: name, ReadOnly: true,
			Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
				n := atomic.AddInt64(&active, 1)
				for {
					p := atomic.LoadInt64(&peak)
					if n <= p || atomic.CompareAndSwapInt64(&peak, p, n) {
						break
					}
				}
				time.Sleep(120 * time.Millisecond)
				atomic.AddInt64(&active, -1)
				return name + "-done", nil
			},
		}
	}
	if err := reg.Register(mk("read_a")); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(mk("read_b")); err != nil {
		t.Fatal(err)
	}
	if err := reg.Allow(CopilotRole, "read_a", "read_b"); err != nil {
		t.Fatal(err)
	}
	s := &Server{toolRegistry: reg}
	calls := []llm.ToolCall{{ID: "1", Name: "read_a"}, {ID: "2", Name: "read_b"}}
	if !s.allToolCallsReadOnly(calls) {
		t.Fatal("read-only calls must be eligible")
	}
	start := time.Now()
	res := s.dispatchReadOnlyTools(context.Background(), calls)
	elapsed := time.Since(start)
	if elapsed > 220*time.Millisecond {
		t.Fatalf("elapsed=%v: calls did not overlap", elapsed)
	}
	if atomic.LoadInt64(&peak) < 2 {
		t.Fatalf("peak concurrency=%d", peak)
	}
	for i, r := range res {
		if r.err != nil || r.out == nil {
			t.Fatalf("call %d failed: %+v", i, r)
		}
	}
	if s.allToolCallsReadOnly([]llm.ToolCall{{Name: "submit_local_sim"}}) {
		t.Fatal("simulator submit must never be parallelized")
	}
}
