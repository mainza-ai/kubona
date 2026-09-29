---
name: Confidentiality Lattice over the Provenance DAG
type: concept
tags: [frontier, security, provenance, ifc, multi-tenant, export-control]
date: 2026-09-27
updated: 2026-09-27
status: open-question
sources: [source-frontier-repo-audit-2026]
---

# Confidentiality Lattice over the Provenance DAG

**Definition.** Attach a **confidentiality label** to every ledger record, let labels
*propagate forward through the provenance DAG by lattice join*, and refuse any read, export, or
downstream job whose requester cannot satisfy the resulting label. Kubona's ledger currently
answers "where did this number come from". This adds the second question a real lab must
answer: **"who is allowed to see it, and did that answer itself move when the data did?"**

## Why provenance alone is insufficient

[[provenance-and-reproducibility]] gives lineage; lineage makes confidentiality *harder*, not
easier — a record that cites an ITAR-flagged device manual and a public preprint must not be
exportable just because the preprint is. Today a `query_records` read is authorized by tenant,
not by content, so a single over-broad grant leaks the entire transitive closure of a result.
That is the exact failure mode a research platform cannot afford: mixed-publication-licence
literature, sponsor NDAs, and export-controlled hardware specs all land in the same DAG.

## Verified primitive: `buzz/crates/ifc-core` (Apache-2.0)

From `crates/ifc-core/src/lib.rs` ((repo-audit-2026)) — a working information-flow lattice, not
a diagram:

```rust
struct ConfidentialityLabel { parties: ReaderSet }   // readers, not secrecy-levels
fn join(&self, other) -> Self                        // least upper bound = intersection of readers
impl PartialOrder { fn partial_cmp(...) }            // finer ⊑ coarser
fn can_flow_to(&self, dst: &Self) -> bool
fn validate_writers(assignment: &WriterAssignment) -> Result<(), Vec<SecurityViolation>>
```

The reader-set formulation is the right one for research: "readable by ∅" means *published*,
"readable by {sponsor-X}" means a single-party NDA, and **join = intersection of readers**, so
mixing two sources can only shrink the audience. `buzz` also ships `crates/ifc-plugin` that
enforces the policy from a signed `pack.toml`, and its whole coordination layer is a signed
event log ("a Nostr relay is a durable, signed, append-only event log", `ARCHITECTURE.md`) —
i.e. labels travel with events and are attributable to a key, not to a session.

## Mechanism

1. **Label at ingest.** Every `raw/` document, provider manual, dataset, and run gets a
   `ReaderSet`: `{public}` · `{tenant:acme}` · `{party:nus}` · `{internal:kubona}`. Licence and
   export flags from [[science-data-integrations]] seed it (a non-commercial-licensed paper
   cannot yield a `{public}` derivative *weight table*, only a `{internal:kubona}` one).
2. **Join on derivation.** `write_record` computes
   `label(new) = ⨝ label(inputs) ∪ label(policy_tag)` from the record's declared inputs. Because
   the ledger is append-only, this is auditable after the fact and cannot be laundered: a
   record cannot *become* more public by being copied (mirrors `can_flow_to`'s one-way check).
3. **Enforce at three seams.** `query_records` (filter by requester's readership *before*
   serialisation — filtering after flooding wastes budget and leaks existence), artefact export
   ([[agent-ux-control-plane]]), and job admission (a job whose inputs include `{party:nus}` may
   not run with an egress-capable provider unless a grant exists).
4. **Covert-channel budget.** A record's *existence* is information. Aggregate counts and
   metrics emitted by [[evaluation-and-benchmarks]] are label-joined too, which is why
   dashboards must be label-scoped rather than global-with-redaction.
5. **Attribution separate from secrecy.** `buzz` distinguishes *who wrote it*
   (`validate_writers`) from *who may read it*; Kubona keeps both — authorship is signed,
   readership is a lattice (see [[user-identity-and-profiles]]).

## What is genuinely new

Publishing a **labelled provenance DAG** is something no quantum cloud console does: a user can
hand a collaborator a *sub-DAG* rather than a PDF, and the confidentiality boundary survives the
handoff because it is computed, not asserted. It also makes licence compliance mechanical for the
model-distillation problem in [[meta-search-and-self-evolution]]: training-data labels become an
admission condition for GQE, so "we distilled a non-commercial corpus into a shipped checkpoint"
becomes a build failure instead of a lawyer's surprise.

## Boundary conditions

Reader-set join is a *complete* lattice only for the policies Kubona supports: parties, tenant,
public. Purpose binding ("research only", "no benchmarking disclosure") and time-bounded NDA
expiry are **not** expressible as reader sets — those need either purpose tags beside the label
or a delegation expiry, and the system must be able to *refuse* a label it cannot model rather
than approximate one. Enforcement stays in the query/export layer (Rust core, tested);
`buzz`-style signed events are the transport, not the authority.

See also [[secrets-and-workload-isolation]], [[provenance-and-reproducibility]],
[[buzz]], [[multi-agent-research-collective]], [[user-identity-and-profiles]].
