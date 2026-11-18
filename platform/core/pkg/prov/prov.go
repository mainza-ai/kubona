package prov

// Provenance export (AG-6b).
//
// Two interoperable artifacts are produced per manuscript:
//
//   1. a W3C PROV-JSON document describing the agents, activities, entities,
//      and the relations between them (what was used, what generated what, who
//      was responsible);
//   2. CRediT-style contributor attribution for declared authors, the Kubona
//      software, the bridge, the operator key, and (only when the ledger says
//      so) an LLM proposer.
//
// The exporter only states what the ledger records. It does not infer that a
// declared author is human, and it does not attribute an LLM to a manuscript
// unless a hypothesis with source="llm" exists in the session. Those limits
// are part of the contract, not caveats hidden in prose.

import (
	"fmt"
	"sort"
	"strings"
)

// Contributor is one CRediT-attributed participant.
type Contributor struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Type  string   `json:"type"`
	Roles []string `json:"credit_roles"`
	Note  string   `json:"note,omitempty"`
}

// CRediT role names (the official vocabulary uses en dashes).
const (
	RoleConceptualization = "Conceptualization"
	RoleMethodology       = "Methodology"
	RoleSoftware          = "Software"
	RoleInvestigation     = "Investigation"
	RoleDataCuration      = "Data Curation"
	RoleWritingOriginal   = "Writing – original draft"
	RoleWritingReview     = "Writing – review & editing"
	RoleSupervision       = "Supervision"
)

type Claim struct {
	ID        string
	Statement string
	Verdict   string
	CreatedAt string
	JobID     string
}

type Job struct {
	ID          string
	Type        string
	Status      string
	CreatedAt   string
	CompletedAt string
}

type Hypothesis struct {
	ID        string
	Statement string
	Status    string
	Source    string
	ClaimID   string
}

type Plan struct {
	ID           string
	HypothesisID string
	Status       string
	JobID        string
}

type Verification struct {
	Status    string
	CheckedAt string
}

type Signoff struct {
	Signer      string
	SignedAt    string
	ContentHash string
}

// Bundle is the ledger-derived input to the export.
type Bundle struct {
	SessionID    string
	ManuscriptID string
	Title        string
	ContentHash  string
	CreatedAt    string
	Authors      []string
	Claims       []Claim
	Jobs         []Job
	Hypotheses   []Hypothesis
	Plans        []Plan
	Verification *Verification
	Signoff      *Signoff
}

func manuscriptID(b Bundle) string { return "kubona:manuscript/" + b.ManuscriptID }
func campaignID(b Bundle) string   { return "kubona:campaign/" + b.SessionID }
func jobID(id string) string       { return "kubona:job/" + id }
func runID(id string) string       { return "kubona:run/" + id }
func claimID(id string) string     { return "kubona:claim/" + id }
func verifyID(b Bundle) string     { return "kubona:verification/" + b.ManuscriptID }
func composeID(b Bundle) string    { return "kubona:compose/" + b.ManuscriptID }
func signActivityID(b Bundle) string {
	return "kubona:sign/" + b.ManuscriptID
}

const (
	agentCore     = "kubona:software/kubona-core"
	agentBridge   = "kubona:software/kubona-bridge"
	agentOperator = "kubona:human/operator"
	agentLLM      = "kubona:model/llm-proposer"
)

// Contributors returns the attributed participants in stable order.
func Contributors(b Bundle) []Contributor {
	out := make([]Contributor, 0, len(b.Authors)+4)
	for _, name := range b.Authors {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		out = append(out, Contributor{
			ID: "kubona:author/" + slug(name), Name: name, Type: "declared-author",
			Roles: []string{RoleConceptualization, RoleWritingOriginal},
			Note:  "declared by the manuscript author list; personhood is not asserted by the exporter",
		})
	}
	out = append(out, Contributor{
		ID: agentCore, Name: "Kubona Core", Type: "software",
		Roles: []string{RoleSoftware, RoleMethodology, RoleDataCuration},
	})
	if len(b.Jobs) > 0 {
		out = append(out, Contributor{
			ID: agentBridge, Name: "Kubona quantum bridge", Type: "software",
			Roles: []string{RoleInvestigation, RoleDataCuration},
		})
	}
	if hasLLMHypothesis(b) {
		out = append(out, Contributor{
			ID: agentLLM, Name: "Configured LLM proposer", Type: "model",
			Roles: []string{RoleInvestigation},
			Note:  "attributed because at least one hypothesis in the session has source=llm",
		})
	}
	if b.Signoff != nil {
		out = append(out, Contributor{
			ID: agentOperator, Name: "Operator (reviewer key)", Type: "human-operator",
			Roles: []string{RoleSupervision, RoleWritingReview},
			Note:  "signer key " + shortKey(b.Signoff.Signer),
		})
	}
	return out
}

// BuildPROVJSON renders W3C PROV-JSON for the bundle.
func BuildPROVJSON(b Bundle) map[string]any {
	doc := map[string]any{
		"prefix": map[string]any{
			"prov":   "http://www.w3.org/ns/prov#",
			"kubona": "https://kubona.ai/ns#",
			"xsd":    "http://www.w3.org/2001/XMLSchema#",
		},
	}
	entity := map[string]any{
		manuscriptID(b): map[string]any{
			"prov:type":          "kubona:Manuscript",
			"prov:label":         b.Title,
			"kubona:contentHash": b.ContentHash,
			"prov:generatedAtTime": b.CreatedAt,
		},
		campaignID(b): map[string]any{
			"prov:type":  "kubona:CampaignSnapshot",
			"prov:label": "session " + b.SessionID,
		},
	}
	activity := map[string]any{
		composeID(b): map[string]any{
			"prov:type":      "kubona:ManuscriptComposition",
			"prov:startTime": b.CreatedAt,
		},
	}
	agent := map[string]any{
		agentCore: map[string]any{
			"prov:type": "prov:SoftwareAgent", "prov:label": "Kubona Core",
		},
	}
	wasGeneratedBy := map[string]any{
		"kubona:gen/" + b.ManuscriptID: map[string]any{
			"prov:entity": manuscriptID(b), "prov:activity": composeID(b), "prov:time": b.CreatedAt,
		},
	}
	used := map[string]any{
		"kubona:use/compose": map[string]any{
			"prov:activity": composeID(b), "prov:entity": campaignID(b),
		},
	}
	wasAssociatedWith := map[string]any{
		"kubona:assoc/compose": map[string]any{
			"prov:activity": composeID(b), "prov:agent": agentCore,
			"prov:role": "manuscript composition",
		},
		"kubona:assoc/core": map[string]any{
			"prov:activity": composeID(b), "prov:agent": agentCore,
		},
	}
	wasAttributedTo := map[string]any{}
	wasDerivedFrom := map[string]any{
		"kubona:derived/manuscript": map[string]any{
			"prov:generatedEntity": manuscriptID(b), "prov:usedEntity": campaignID(b),
		},
	}
	for _, a := range Contributors(b) {
		switch a.Type {
		case "declared-author":
			agent[a.ID] = map[string]any{
				"prov:type": "prov:Agent", "prov:label": a.Name,
				"kubona:contributorType": a.Type, "kubona:creditRole": a.Roles,
			}
			wasAttributedTo["kubona:attr/"+slug(a.Name)] = map[string]any{
				"prov:entity": manuscriptID(b), "prov:agent": a.ID,
			}
		case "model":
			agent[a.ID] = map[string]any{
				"prov:type": "prov:SoftwareAgent", "prov:label": a.Name,
				"kubona:contributorType": a.Type, "kubona:creditRole": a.Roles,
			}
		case "human-operator":
			agent[a.ID] = map[string]any{
				"prov:type": "prov:Person", "prov:label": a.Name,
				"kubona:contributorType": a.Type, "kubona:creditRole": a.Roles,
			}
		}
	}

	if len(b.Jobs) > 0 {
		agent[agentBridge] = map[string]any{
			"prov:type": "prov:SoftwareAgent", "prov:label": "Kubona quantum bridge",
		}
	}
	for _, j := range b.Jobs {
		entity[jobID(j.ID)] = map[string]any{
			"prov:type": "kubona:JobResult", "prov:label": j.Type,
			"kubona:status": j.Status, "prov:generatedAtTime": j.CreatedAt,
		}
		activity[runID(j.ID)] = map[string]any{
			"prov:type": "kubona:ExperimentRun", "prov:startTime": j.CreatedAt,
			"prov:endTime": j.CompletedAt,
		}
		wasGeneratedBy["kubona:gen/"+j.ID] = map[string]any{
			"prov:entity": jobID(j.ID), "prov:activity": runID(j.ID), "prov:time": j.CompletedAt,
		}
		used["kubona:use/"+j.ID] = map[string]any{
			"prov:activity": runID(j.ID), "prov:entity": campaignID(b),
		}
		wasAssociatedWith["kubona:assoc/"+j.ID] = map[string]any{
			"prov:activity": runID(j.ID), "prov:agent": agentBridge, "prov:role": "experiment execution",
		}
	}
	for _, c := range b.Claims {
		entity[claimID(c.ID)] = map[string]any{
			"prov:type": "kubona:Claim", "prov:label": c.Statement,
			"kubona:verdict": c.Verdict, "prov:generatedAtTime": c.CreatedAt,
		}
		if c.JobID != "" {
			wasDerivedFrom["kubona:derived/"+c.ID] = map[string]any{
				"prov:generatedEntity": claimID(c.ID), "prov:usedEntity": jobID(c.JobID),
			}
			wasGeneratedBy["kubona:gen/"+c.ID] = map[string]any{
				"prov:entity": claimID(c.ID), "prov:activity": runID(c.JobID), "prov:time": c.CreatedAt,
			}
		}
	}
	for _, h := range b.Hypotheses {
		entity["kubona:hypothesis/"+h.ID] = map[string]any{
			"prov:type": "kubona:Hypothesis", "prov:label": h.Statement,
			"kubona:status": h.Status, "kubona:source": h.Source,
		}
		if h.ClaimID != "" {
			wasDerivedFrom["kubona:derived/hyp/"+h.ID] = map[string]any{
				"prov:generatedEntity": claimID(h.ClaimID),
				"prov:usedEntity":      "kubona:hypothesis/" + h.ID,
			}
		}
	}
	for _, p := range b.Plans {
		entity["kubona:plan/"+p.ID] = map[string]any{
			"prov:type": "kubona:ExperimentPlan", "prov:label": p.ID,
			"kubona:status": p.Status,
		}
		if p.JobID != "" {
			wasDerivedFrom["kubona:derived/plan/"+p.ID] = map[string]any{
				"prov:generatedEntity": jobID(p.JobID), "prov:usedEntity": "kubona:plan/" + p.ID,
			}
		}
	}
	if b.Verification != nil {
		entity[verifyID(b)] = map[string]any{
			"prov:type": "kubona:VerificationReport", "kubona:status": b.Verification.Status,
			"prov:generatedAtTime": b.Verification.CheckedAt,
		}
		activity["kubona:verify/"+b.ManuscriptID] = map[string]any{
			"prov:type": "kubona:EvidenceVerification", "prov:startTime": b.Verification.CheckedAt,
		}
		wasGeneratedBy["kubona:gen/verify"] = map[string]any{
			"prov:entity": verifyID(b), "prov:activity": "kubona:verify/" + b.ManuscriptID,
			"prov:time": b.Verification.CheckedAt,
		}
		used["kubona:use/verify"] = map[string]any{
			"prov:activity": "kubona:verify/" + b.ManuscriptID, "prov:entity": manuscriptID(b),
		}
		wasAssociatedWith["kubona:assoc/verify"] = map[string]any{
			"prov:activity": "kubona:verify/" + b.ManuscriptID, "prov:agent": agentCore,
			"prov:role": "evidence verification",
		}
		wasDerivedFrom["kubona:derived/verify"] = map[string]any{
			"prov:generatedEntity": verifyID(b), "prov:usedEntity": manuscriptID(b),
		}
	}
	if b.Signoff != nil {
		activity[signActivityID(b)] = map[string]any{
			"prov:type": "kubona:ManuscriptSignoff", "prov:startTime": b.Signoff.SignedAt,
		}
		used["kubona:use/sign"] = map[string]any{
			"prov:activity": signActivityID(b), "prov:entity": manuscriptID(b),
		}
		wasAssociatedWith["kubona:assoc/sign"] = map[string]any{
			"prov:activity": signActivityID(b), "prov:agent": agentOperator,
			"prov:role": "signoff",
		}
	}

	doc["entity"] = entity
	doc["activity"] = activity
	doc["agent"] = agent
	doc["wasGeneratedBy"] = wasGeneratedBy
	doc["used"] = used
	doc["wasAssociatedWith"] = wasAssociatedWith
	if len(wasAttributedTo) > 0 {
		doc["wasAttributedTo"] = wasAttributedTo
	}
	doc["wasDerivedFrom"] = wasDerivedFrom
	return doc
}

// ProvOGraph returns JSON-LD nodes for the RO-Crate @graph. Relations that
// PROV-O expresses on the subject are folded onto the subject node.
func ProvOGraph(b Bundle) []map[string]any {
	nodes := []map[string]any{
		{
			"@id": agentCore, "@type": "prov:SoftwareAgent", "name": "Kubona Core",
			"kubona:creditRole": []string{RoleSoftware, RoleMethodology, RoleDataCuration},
		},
		{
			"@id": campaignID(b), "@type": "prov:Entity", "name": "campaign " + b.SessionID,
		},
		{
			"@id": composeID(b), "@type": "prov:Activity", "name": "manuscript composition",
			"prov:used":         []map[string]any{{"@id": campaignID(b)}},
			"prov:wasAssociatedWith": []map[string]any{{"@id": agentCore}},
		},
		{
			"@id": manuscriptID(b), "@type": []string{"prov:Entity", "kubona:Manuscript"},
			"name": b.Title,
			"prov:wasGeneratedBy": []map[string]any{{"@id": composeID(b)}},
			"prov:wasDerivedFrom": []map[string]any{{"@id": campaignID(b)}},
			"kubona:contentHash":  b.ContentHash,
		},
	}
	for _, a := range Contributors(b) {
		if a.Type != "declared-author" {
			continue
		}
		nodes = append(nodes, map[string]any{
			"@id": a.ID, "@type": "prov:Agent", "name": a.Name,
			"kubona:contributorType": a.Type, "kubona:creditRole": a.Roles,
		})
	}
	if b.Verification != nil {
		nodes = append(nodes, map[string]any{
			"@id": verifyID(b), "@type": []string{"prov:Entity", "kubona:VerificationReport"},
			"kubona:status":       b.Verification.Status,
			"prov:wasGeneratedBy": []map[string]any{{"@id": "kubona:verify/" + b.ManuscriptID}},
			"prov:wasDerivedFrom": []map[string]any{{"@id": manuscriptID(b)}},
		})
	}
	if b.Signoff != nil {
		nodes = append(nodes, map[string]any{
			"@id": signActivityID(b), "@type": "prov:Activity", "name": "manuscript signoff",
			"prov:used":              []map[string]any{{"@id": manuscriptID(b)}},
			"prov:wasAssociatedWith": []map[string]any{{"@id": agentOperator}},
		})
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		return fmt.Sprint(nodes[i]["@id"]) < fmt.Sprint(nodes[j]["@id"])
	})
	return nodes
}

func hasLLMHypothesis(b Bundle) bool {
	for _, h := range b.Hypotheses {
		if strings.EqualFold(h.Source, "llm") {
			return true
		}
	}
	return false
}

func slug(s string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
		} else if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func shortKey(key string) string {
	if len(key) <= 12 {
		return key
	}
	return key[:12] + "…"
}
