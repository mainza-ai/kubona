package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/mainza-ai/kubona/core/pkg/codesign"
)

// Codesign routes are read-only scoring (no spend, no job submission):
// TQ arithmetic + MAP-Elites frontier cells for the console cockpit.
// Validity ground truth stays Python-owned (kickmix evaluator); Go serves
// numbers only, never verdicts.

// handleCodesignTQ scores one (T,Q,kappa) triple. GET for console reads.
func (s *Server) handleCodesignTQ(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	// A present-but-garbled param refuses (typo'd kappa must 400, never
	// silently score as kappa=1); absent params take the fail-closed
	// defaults that Cost rejects downstream.
	var err error
	t, qq, kappa := -1, -1, codesign.KappaDefault
	if s := q.Get("toffoli"); s != "" {
		if t, err = strconv.Atoi(strings.TrimSpace(s)); err != nil {
			WriteAPIError(w, "bad_request", "toffoli must be an int: refusing", nil)
			return
		}
	}
	if s := q.Get("qubits"); s != "" {
		if qq, err = strconv.Atoi(strings.TrimSpace(s)); err != nil {
			WriteAPIError(w, "bad_request", "qubits must be an int: refusing", nil)
			return
		}
	}
	if s := q.Get("kappa"); s != "" {
		if kappa, err = strconv.Atoi(strings.TrimSpace(s)); err != nil {
			WriteAPIError(w, "bad_request", "kappa must be an int: refusing", nil)
			return
		}
	}
	tq, err := codesign.Cost(t, qq, kappa)
	if err != nil {
		WriteAPIError(w, "bad_request", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"toffoli": t, "qubits": qq, "kappa": kappa, "tq": tq,
		"golden_check": tq == 50 && t == 5 && qq == 10,
	})
}

type codesignEntry struct {
	Toffoli int `json:"toffoli"`
	Qubits  int `json:"qubits"`
}

// handleCodesignFrontier scores a batch (<=64) into MAP-Elites cells.
// POST {entries:[{toffoli,qubits}], kappa}. Invalid entries refuse whole.
func (s *Server) handleCodesignFrontier(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Entries []codesignEntry `json:"entries"`
		Kappa   int             `json:"kappa"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteAPIError(w, "bad_request", "invalid JSON body: refusing", nil)
		return
	}
	if req.Kappa == 0 {
		req.Kappa = codesign.KappaDefault
	}
	if len(req.Entries) > 64 {
		WriteAPIError(w, "bad_request", "entries must be <=64: refusing", nil)
		return
	}
	f := codesign.NewFrontier(12, 12, req.Kappa)
	type scored struct {
		Toffoli int   `json:"toffoli"`
		Qubits  int   `json:"qubits"`
		TQ      int64 `json:"tq"`
	}
	var out []scored
	for _, e := range req.Entries {
		tq, err := codesign.Cost(e.Toffoli, e.Qubits, f.Kappa)
		if err != nil {
			WriteAPIError(w, "bad_request", err.Error(), nil)
			return
		}
		if _, err := f.Offer(e.Toffoli, e.Qubits); err != nil {
			WriteAPIError(w, "bad_request", err.Error(), nil)
			return
		}
		out = append(out, scored{e.Toffoli, e.Qubits, tq})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TQ < out[j].TQ })
	best := f.Best()
	writeJSON(w, http.StatusOK, map[string]any{
		"kappa": req.Kappa, "cells": len(f.Entries()),
		"best": best, "entries": out,
	})
}


// handleCodesignVerified serves the versioned verified-results document
// (G4). Read-only GET; embedded JSON served verbatim (numbers, never
// verdicts). Shape drift refuses loudly via ParseVerifiedReport.
func (s *Server) handleCodesignVerified(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed: refusing", http.StatusMethodNotAllowed)
		return
	}
	rep, err := codesign.ParseVerifiedReport()
	if err != nil {
		http.Error(w, fmt.Sprintf("verified results unavailable: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}
