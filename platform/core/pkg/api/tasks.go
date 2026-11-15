package api

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

// Tasks are session-owned follow-up records. The workbench stores only the
// task id; content lives in this table, never in the layout document.
func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
		if sessionID == "" {
			WriteAPIError(w, "bad_request", "session_id is required", nil)
			return
		}
		if _, ok := s.authorizeKnownSession(w, r, sessionID); !ok {
			return
		}
		tasks, err := s.ledger.ListTasks(sessionID)
		if err != nil {
			WriteAPIError(w, "internal", "task list failed", nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks})
	case http.MethodPost:
		var req struct {
			SessionID    string `json:"session_id"`
			Title        string `json:"title"`
			SourceDomain string `json:"source_domain"`
			SourceID     string `json:"source_id"`
			RequestID    string `json:"request_id"`
		}
		if err := decodeStrict(r, &req); err != nil {
			WriteAPIError(w, "bad_request", fmt.Sprintf("invalid request body: %v", err), nil)
			return
		}
		req.SessionID = strings.TrimSpace(req.SessionID)
		req.Title = strings.TrimSpace(req.Title)
		if req.SessionID == "" || req.Title == "" || len(req.Title) > 200 {
			WriteAPIError(w, "bad_request", "session_id and a title (<=200 chars) are required", nil)
			return
		}
		resolved, serr := s.requireSession(req.SessionID)
		if serr != nil {
			WriteAPIError(w, "not_found", serr.Error(), nil)
			return
		}
		req.SessionID = resolved
		if _, ok := s.authorizeKnownSession(w, r, req.SessionID); !ok {
			return
		}
		ticket, refusal := s.idemBegin(req.SessionID, req.RequestID, map[string]any{
			"title": req.Title, "source_domain": req.SourceDomain, "source_id": req.SourceID,
		}, idemRailJob)
		if refusal != nil {
			refusal.write(w)
			return
		}
		if ticket.replay {
			s.idemReplay(w, ticket)
			return
		}
		task, err := s.ledger.CreateTask(req.SessionID, req.Title, strings.TrimSpace(req.SourceDomain), strings.TrimSpace(req.SourceID))
		if err != nil {
			s.idemFinishError(w, ticket, http.StatusInternalServerError, "task create failed")
			return
		}
		if err := s.ledger.AppendEvent(req.SessionID, "task_created", map[string]any{
			"task_id": task.ID, "title": task.Title,
			"source_domain": task.SourceDomain, "source_id": task.SourceID,
		}); err != nil {
			s.idemFinishError(w, ticket, http.StatusInternalServerError, "task recorded but ledger append failed")
			return
		}
		s.idemFinishJSON(w, ticket, http.StatusOK, task)
	default:
		WriteAPIError(w, "method_not_allowed", "GET or POST required for tasks", nil)
	}
}

func (s *Server) handleTaskByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "task status is POST-only", nil)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
	if id == "" || strings.Contains(id, "/") {
		WriteAPIError(w, "bad_request", "task path is /api/tasks/{id}", nil)
		return
	}
	var req struct {
		SessionID string `json:"session_id"`
		Status    string `json:"status"`
		RequestID string `json:"request_id"`
	}
	if err := decodeStrict(r, &req); err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid request body: %v", err), nil)
		return
	}
	if req.SessionID == "" {
		WriteAPIError(w, "bad_request", "session_id is required", nil)
		return
	}
	if _, ok := s.authorizeKnownSession(w, r, req.SessionID); !ok {
		return
	}
	ticket, refusal := s.idemBegin(req.SessionID, req.RequestID, map[string]any{
		"task_id": id, "status": req.Status,
	}, idemRailJob)
	if refusal != nil {
		refusal.write(w)
		return
	}
	if ticket.replay {
		s.idemReplay(w, ticket)
		return
	}
	task, err := s.ledger.UpdateTaskStatus(id, req.SessionID, req.Status)
	if err == sql.ErrNoRows {
		s.idemFinishError(w, ticket, http.StatusNotFound, "task not found in this session")
		return
	}
	if err != nil {
		s.idemFinishError(w, ticket, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.ledger.AppendEvent(req.SessionID, "task_status_changed", map[string]any{"task_id": task.ID, "status": task.Status}); err != nil {
		s.idemFinishError(w, ticket, http.StatusInternalServerError, "task updated but ledger append failed")
		return
	}
	s.idemFinishJSON(w, ticket, http.StatusOK, task)
}

// handleTaskBatchRoute decodes the batch shape on its own path; a distinct
// path keeps one response contract per endpoint (single task vs task list).
func (s *Server) handleTaskBatchRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "batch task creation is POST-only", nil)
		return
	}
	var req struct {
		SessionID string `json:"session_id"`
		RequestID string `json:"request_id"`
		Tasks     []struct {
			Title        string `json:"title"`
			SourceDomain string `json:"source_domain"`
			SourceID     string `json:"source_id"`
			Order        int    `json:"order"`
		} `json:"tasks"`
	}
	if err := decodeStrict(r, &req); err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid request body: %v", err), nil)
		return
	}
	s.handleTaskBatch(w, r, req.SessionID, req.RequestID, req.Tasks)
}

// handleTaskBatch creates tasks in plan order under one idempotency key. A
// replay returns the same created rows, so a double-click cannot duplicate a
// plan. Batch is bounded; the order is the request order (or `order`).
func (s *Server) handleTaskBatch(w http.ResponseWriter, r *http.Request, sessionID, requestID string, items []struct {
	Title        string `json:"title"`
	SourceDomain string `json:"source_domain"`
	SourceID     string `json:"source_id"`
	Order        int    `json:"order"`
}) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		WriteAPIError(w, "bad_request", "session_id is required", nil)
		return
	}
	if len(items) > 50 {
		WriteAPIError(w, "bad_request", "at most 50 tasks may be created per request", nil)
		return
	}
	for _, item := range items {
		title := strings.TrimSpace(item.Title)
		if title == "" || len(title) > 200 {
			WriteAPIError(w, "bad_request", "every task needs a title (<=200 chars)", nil)
			return
		}
	}
	resolved, serr := s.requireSession(sessionID)
	if serr != nil {
		WriteAPIError(w, "not_found", serr.Error(), nil)
		return
	}
	sessionID = resolved
	if _, ok := s.authorizeKnownSession(w, r, sessionID); !ok {
		return
	}
	ordered := make([]struct {
		Title        string `json:"title"`
		SourceDomain string `json:"source_domain"`
		SourceID     string `json:"source_id"`
		Order        int    `json:"order"`
	}, len(items))
	copy(ordered, items)
	// Stable: supplied order wins, array order breaks ties.
	for i := 1; i < len(ordered); i++ {
		for j := i; j > 0 && ordered[j].Order < ordered[j-1].Order; j-- {
			ordered[j], ordered[j-1] = ordered[j-1], ordered[j]
		}
	}
	content := map[string]any{"tasks": ordered}
	ticket, refusal := s.idemBegin(sessionID, requestID, content, idemRailJob)
	if refusal != nil {
		refusal.write(w)
		return
	}
	if ticket.replay {
		s.idemReplay(w, ticket)
		return
	}
	created := make([]ledger.Task, 0, len(ordered))
	rollbackCreated := func() {
		for _, t := range created {
			_, _ = s.ledger.DB().Exec(`DELETE FROM tasks WHERE id = ?`, t.ID)
		}
	}
	for _, item := range ordered {
		task, err := s.ledger.CreateTask(sessionID, strings.TrimSpace(item.Title),
			strings.TrimSpace(item.SourceDomain), strings.TrimSpace(item.SourceID))
		if err != nil {
			rollbackCreated()
			s.idemFinishError(w, ticket, http.StatusInternalServerError, "batch task create failed")
			return
		}
		created = append(created, task)
		if err := s.ledger.AppendEvent(sessionID, "task_created", map[string]any{
			"task_id": task.ID, "title": task.Title,
			"source_domain": task.SourceDomain, "source_id": task.SourceID,
		}); err != nil {
			rollbackCreated()
			s.idemFinishError(w, ticket, http.StatusInternalServerError, "tasks recorded but ledger append failed")
			return
		}
	}
	s.idemFinishJSON(w, ticket, http.StatusOK, map[string]any{"tasks": created, "count": len(created)})
}
