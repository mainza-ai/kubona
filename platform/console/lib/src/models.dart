/// Mirror of the platform MCP tool payloads (single source of truth: the
/// kubona.mcp.servers modules). Keep these in lockstep with the server schemas.

class Session {
  final String sessionId;
  final String name;
  final String owner;
  final double? budgetCeilingUsd;
  final double spentUsd;
  final String status;

  const Session({
    required this.sessionId,
    required this.name,
    required this.owner,
    this.budgetCeilingUsd,
    required this.spentUsd,
    required this.status,
  });

  factory Session.fromJson(Map<String, dynamic> j) => Session(
        sessionId: j['session_id'] as String,
        name: j['name'] as String,
        owner: j['owner'] as String,
        budgetCeilingUsd: (j['budget_ceiling_usd'] as num?)?.toDouble(),
        spentUsd: (j['spent_usd'] as num).toDouble(),
        status: j['status'] as String,
      );

  double? get remainingUsd =>
      budgetCeilingUsd == null ? null : budgetCeilingUsd! - spentUsd;
}

class Approval {
  final String approvalId;
  final String sessionId;
  final String state; // REVIEW | HOLD | APPROVED | REJECTED
  final double amountRequestedUsd;
  final String reason;

  const Approval({
    required this.approvalId,
    required this.sessionId,
    required this.state,
    required this.amountRequestedUsd,
    required this.reason,
  });

  factory Approval.fromJson(Map<String, dynamic> j) => Approval(
        approvalId: j['approval_id'] as String,
        sessionId: j['session_id'] as String,
        state: j['state'] as String,
        amountRequestedUsd: (j['amount_requested_usd'] as num).toDouble(),
        reason: j['reason'] as String,
      );
}

class Loop {
  final String loopId;
  final String name;
  final String status; // running | paused | completed | killed
  final int step;
  final int totalSteps;
  final String currentStepDesc;

  const Loop({
    required this.loopId,
    required this.name,
    required this.status,
    required this.step,
    required this.totalSteps,
    required this.currentStepDesc,
  });

  factory Loop.fromJson(Map<String, dynamic> j) => Loop(
        loopId: j['loop_id'] as String,
        name: j['name'] as String,
        status: j['status'] as String,
        step: j['step'] as int,
        totalSteps: j['total_steps'] as int,
        currentStepDesc: j['current_step_desc'] as String,
      );
}

class ProviderCapability {
  final String provider;
  final String modality;
  final String status;
  final String pricingModel;

  const ProviderCapability({
    required this.provider,
    required this.modality,
    required this.status,
    required this.pricingModel,
  });

  factory ProviderCapability.fromJson(Map<String, dynamic> j) => ProviderCapability(
        provider: j['provider'] as String,
        modality: (j['modality'] as Map<String, dynamic>)['value'] as String,
        status: j['status'] as String,
        pricingModel: (j['pricing'] as Map<String, dynamic>)['model'] as String,
      );
}

class CircuitDescriptor {
  final String circuitName;
  final int numQubits;
  final int depth;
  final String circuitHash;
  final String canonicalQasm;

  const CircuitDescriptor({
    required this.circuitName,
    required this.numQubits,
    required this.depth,
    required this.circuitHash,
    required this.canonicalQasm,
  });

  factory CircuitDescriptor.fromJson(Map<String, dynamic> j) => CircuitDescriptor(
        circuitName: j['circuit_name'] as String,
        numQubits: j['num_qubits'] as int,
        depth: j['depth'] as int,
        circuitHash: j['circuit_hash'] as String,
        canonicalQasm: j['canonical_qasm'] as String,
      );
}
