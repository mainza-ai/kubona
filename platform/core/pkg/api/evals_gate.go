package api

import (
	"fmt"
	"os"
	"strings"

	"github.com/mainza-ai/kubona/core/pkg/evals"
	"github.com/mainza-ai/kubona/core/pkg/llm"
)

// B12: an opt-in gate on copilot model changes. Default off because a fresh
// install has no recorded eval and must still be configurable; deployments
// that treat decision quality as a release gate set
// KUBONA_REQUIRE_LLM_EVAL=1. A change to an unevaluated (or stale) model is
// refused unless the operator explicitly acknowledges it in the same write.
const requireLLMEvalEnv = "KUBONA_REQUIRE_LLM_EVAL"

type llmEvalAck struct {
	Provider string
	Model    string
	Status   string
}

type llmEvalRefusal struct {
	Status evals.Status
}

func (e *llmEvalRefusal) Error() string {
	return fmt.Sprintf("copilot model %s/%s is %s on the pinned fixtures", e.Status.Provider, e.Status.Model, e.Status.Status)
}

// enforceLLMEvalGate inspects a settings write before it commits. It only
// gates a changed copilot assignment; unrelated writes pass untouched.
func (s *Server) enforceLLMEvalGate(req *llm.Settings, acknowledged bool) (*llmEvalAck, error) {
	if strings.TrimSpace(os.Getenv(requireLLMEvalEnv)) != "1" {
		return nil, nil
	}
	if req == nil || req.RoleAssignments == nil || s.llmSettings == nil {
		return nil, nil
	}
	next, ok := req.RoleAssignments["copilot"]
	if !ok {
		return nil, nil
	}
	current := s.llmSettings.GetSnapshot().RoleAssignments["copilot"]
	if next.Provider == current.Provider && next.Model == current.Model {
		return nil, nil
	}
	set, err := evals.Load("copilot")
	if err != nil {
		// Fixtures are embedded; an unreadable set is a build fault, and
		// admitting a model without the gate would be worse than refusing.
		return nil, fmt.Errorf("eval fixtures unavailable: %w", err)
	}
	status := s.evalStore.Status(set, "copilot", string(next.Provider), next.Model)
	if status.Status == evals.StatusEvaluated {
		return nil, nil
	}
	if acknowledged {
		return &llmEvalAck{Provider: string(next.Provider), Model: next.Model, Status: status.Status}, nil
	}
	return nil, &llmEvalRefusal{Status: status}
}
