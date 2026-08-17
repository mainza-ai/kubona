/// The two Phase-2 console surfaces (plan §5.3 priorities) as data contracts.
///
/// Flutter widget implementations live under lib/ui/ on a Flutter-equipped
/// machine; these classes fix what each surface MUST show so the widget layer
/// stays a thin renderer (per the ui-ux-vision: summaries first, drill-down).

/// Surface 1 — cost & approval console (burn-rate, queue, ceilings).
class CostApprovalSnapshot {
  final double spentUsd;
  final double? ceilingUsd;
  final double? remainingUsd;
  final double burnRateUsdPerSec;
  final List<Approval> pendingApprovals;

  const CostApprovalSnapshot({
    required this.spentUsd,
    this.ceilingUsd,
    this.remainingUsd,
    required this.burnRateUsdPerSec,
    required this.pendingApprovals,
  });

  bool get needsAttention => pendingApprovals.isNotEmpty;
  bool get breached => ceilingUsd != null && spentUsd > ceilingUsd!;
}

/// Surface 2 — circuit visualizer (SDK-independent, code<->diagram sync).
class CircuitView {
  final CircuitDescriptor descriptor;
  final List<String> nativeGateCounts; // "h": 4, ...

  const CircuitView({required this.descriptor, required this.nativeGateCounts});

  int get collapsedGateCount => descriptor.depth; // progressive disclosure
}
