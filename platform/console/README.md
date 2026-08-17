# Kubona Console (Dart/Flutter) — Phase 2 scaffold

The control plane per plan §5.3 and the full spec in `wiki/concepts/ui-ux-vision.md`.
**Phase 2 ships the two priority surfaces**: cost & approval console and circuit
visualizer (provenance browser + fleet status follow in Phase 3; experiment
notebook + lab/workflows/agents in Phase 6).

## What's here

| Path | What it is |
|---|---|
| `lib/src/models.dart` | Data contracts mirroring the MCP tool payloads (keep in lockstep with the servers) |
| `lib/src/gateway_client.dart` | JSON-RPC 2.0 client against the gateway (read/approve only; bearer auth) |
| `lib/src/surfaces.dart` | The two Phase-2 surface data contracts (what each MUST show) |
| `pubspec.yaml` | Package manifest (no Flutter SDK needed to author; build requires Flutter) |

## Build

This machine has no Flutter SDK, so the scaffold is authored but not compiled.
On a Flutter-equipped machine:

```bash
cd console
flutter create --project-name kubona_console . --platforms=macos,web,linux,windows
# then implement lib/ui/*.dart widgets against these contracts
```

## Console rules (from ui-ux-vision, non-negotiable)

1. **Read/approve only** — the console never submits jobs; it calls the gateway
   which enforces auth + per-call authorization.
2. **Summaries first** — deep circuits collapse to stats; approval queue is
   one-glance with one-click approve/deny.
3. **Anomaly first** — burn spikes, breach, fidelity drops surface on Home.
4. Mobile = check-in (approve, glance, alert); authoring stays on desktop.
