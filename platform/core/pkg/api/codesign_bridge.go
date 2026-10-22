package api

// T-01b: codesign over the bridge gRPC rail. One RPC with a closed
// operation vocabulary (evaluate/batch/families/packs/budget); the
// operation selects a function in bridge/codesign_ops.py — never
// caller-supplied code. Same transport contract as chemistry:
// transport failure → (nil, false) and the caller falls back to the
// local exec rail; domain refusal arrives as {"ok": false, ...} JSON
// with ok=true, matching the retired snippet shape exactly.

import (
	"context"
	"encoding/json"
	"time"

	pb "github.com/mainza-ai/kubona/core/proto"
)

func (s *Server) codesignViaBridge(ctx context.Context, operation string, requestJSON []byte) ([]byte, bool) {
	bridge := s.jobRunner.BridgeClient()
	if bridge == nil {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	resp, err := bridge.EvaluateCodesign(ctx, &pb.CodesignRequest{
		Operation:   operation,
		RequestJson: string(requestJSON),
	})
	if err != nil || resp == nil {
		return nil, false
	}
	if !resp.Success {
		out, _ := json.Marshal(map[string]any{"ok": false, "error": resp.ErrorMessage})
		return out, true
	}
	return []byte(resp.ResultJson), true
}
