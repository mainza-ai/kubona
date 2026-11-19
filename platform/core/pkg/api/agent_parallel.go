package api

import (
	"context"
	"fmt"
	"sync"

	"github.com/mainza-ai/kubona/core/pkg/llm"
)

type toolExecResult struct {
	out     any
	err     error
	blocked bool
}

// allToolCallsReadOnly reports whether every call is a read-only registry
// tool and none is the per-turn simulator dispatch. Only that set may run
// concurrently; an effectful call keeps the sequential taint barrier.
func (s *Server) allToolCallsReadOnly(calls []llm.ToolCall) bool {
	if s == nil || s.toolRegistry == nil || len(calls) < 2 {
		return false
	}
	for _, c := range calls {
		if c.Name == "submit_local_sim" {
			return false
		}
		def, ok := s.toolRegistry.Definition(c.Name)
		if !ok || !def.ReadOnly {
			return false
		}
	}
	return true
}

// dispatchReadOnlyTools runs independent read-only calls concurrently. The
// observation/audit loop still consumes the results in the model's original
// order, so observation IDs and the transcript stay deterministic.
func (s *Server) dispatchReadOnlyTools(ctx context.Context, calls []llm.ToolCall) []toolExecResult {
	res := make([]toolExecResult, len(calls))
	var wg sync.WaitGroup
	for i, c := range calls {
		wg.Add(1)
		go func(i int, c llm.ToolCall) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					res[i].err = fmt.Errorf("tool %q panicked: %v", c.Name, r)
				}
			}()
			res[i].out, res[i].err = s.toolRegistry.Dispatch(ctx, CopilotRole, toRegistryCall(c))
		}(i, c)
	}
	wg.Wait()
	return res
}
