/// Kubona Observatory — Production Quantum Epistemic Control Plane.
///
/// Implements the 7 core Observatory surfaces:
/// 1. Autonomy Cockpit & Cryptographic AP2 Spend Surface
/// 2. 4D Flight Recorder & Scrubbable Loop Inspector
/// 3. Epistemic Manifold & Classical Anchor Horizon
/// 4. Post-QIR Universal Circuit Visualizer
/// 5. Execution Fleet Continuum Radar
/// 6. Cognitive LLM Mesh & Sovereign Model Control Deck
/// 7. Citable Results Studio & Structured Notebook
///
/// Read/approve only (ui-ux-vision hard rule #1): the console is an oversight
/// layer over autonomous agents, never a parallel job submission path.

import 'package:flutter/material.dart';

import 'src/api_client.dart';
import 'src/theme/observatory_theme.dart';
import 'src/widgets/circuit_canvas.dart';
import 'src/widgets/flight_recorder.dart';
import 'src/widgets/wavefunction_ring_gauge.dart';

void main() {
  runApp(KubonaConsole(api: ConsoleApi()));
}

class KubonaConsole extends StatelessWidget {
  final ConsoleApi api;
  const KubonaConsole({super.key, required this.api});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Kubona Observatory',
      theme: ObservatoryTheme.darkTheme,
      home: ConsoleHome(api: api),
      debugShowCheckedModeBanner: false,
    );
  }
}

class ConsoleHome extends StatefulWidget {
  final ConsoleApi api;
  const ConsoleHome({super.key, required this.api});

  @override
  State<ConsoleHome> createState() => _ConsoleHomeState();
}

class _ConsoleHomeState extends State<ConsoleHome> {
  Future<List<SessionSummary>>? _sessions;
  Future<List<ApprovalItem>>? _approvals;
  Future<List<LoopItem>>? _loops;
  Future<Map<String, dynamic>>? _provenance;
  Future<Map<String, dynamic>>? _fleet;
  Future<List<Map<String, dynamic>>>? _reports;
  Future<List<Map<String, dynamic>>>? _workflows;
  Future<List<Map<String, dynamic>>>? _agents;
  Future<List<Map<String, dynamic>>>? _labs;
  Future<Map<String, dynamic>>? _models;
  Future<Map<String, dynamic>>? _settings;

  @override
  void initState() {
    super.initState();
    _refresh();
  }

  void _refresh() {
    setState(() {
      _sessions = widget.api.sessions();
      _approvals = widget.api.approvals();
      _loops = widget.api.loops();
      _provenance = widget.api.provenance();
      _fleet = widget.api.fleet();
      _reports = widget.api.reports();
      _workflows = widget.api.workflows();
      _agents = widget.api.agents();
      _labs = widget.api.labs();
      _models = widget.api.models();
      _settings = widget.api.settingsGet();
    });
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: ObservatoryTheme.voidBg,
      appBar: AppBar(
        title: Row(
          children: [
            Image.asset(
              'assets/images/logo.png',
              height: 28,
              errorBuilder: (context, error, stackTrace) =>
                  const Icon(Icons.remove_red_eye, color: ObservatoryTheme.cyanCoherence),
            ),
            const SizedBox(width: 12),
            const Text(
              'KUBONA OBSERVATORY',
              style: TextStyle(
                fontFamily: 'monospace',
                fontSize: 16,
                letterSpacing: 1.5,
                fontWeight: FontWeight.bold,
                color: Colors.white,
              ),
            ),
            const SizedBox(width: 16),
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
              decoration: BoxDecoration(
                color: ObservatoryTheme.cyanCoherence.withOpacity(0.12),
                borderRadius: BorderRadius.circular(4),
                border: Border.all(color: ObservatoryTheme.borderCyan),
              ),
              child: const Row(
                children: [
                  Icon(Icons.bolt, size: 12, color: ObservatoryTheme.cyanCoherence),
                  SizedBox(width: 4),
                  Text('SSE LIVE',
                      style: TextStyle(
                          fontSize: 10,
                          fontWeight: FontWeight.bold,
                          color: ObservatoryTheme.cyanCoherence)),
                ],
              ),
            ),
            const SizedBox(width: 8),
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
              decoration: BoxDecoration(
                color: ObservatoryTheme.electricViolet.withOpacity(0.12),
                borderRadius: BorderRadius.circular(4),
                border: Border.all(color: ObservatoryTheme.borderViolet),
              ),
              child: const Text('SOVEREIGN AIR-GAP: ON',
                  style: TextStyle(
                      fontSize: 10,
                      fontWeight: FontWeight.bold,
                      color: ObservatoryTheme.electricViolet)),
            ),
          ],
        ),
        actions: [
          IconButton(
            icon: const Icon(Icons.refresh, color: ObservatoryTheme.cyanCoherence),
            tooltip: 'Synchronize Observatory State',
            onPressed: _refresh,
          ),
          const SizedBox(width: 8),
        ],
      ),
      body: DefaultTabController(
        length: 7,
        child: Column(
          children: [
            Container(
              decoration: const BoxDecoration(
                color: ObservatoryTheme.surface1,
                border: Border(bottom: BorderSide(color: ObservatoryTheme.borderSubtle)),
              ),
              child: const TabBar(
                isScrollable: true,
                tabs: [
                  Tab(icon: Icon(Icons.security, size: 16), text: 'Autonomy Cockpit (AP2)'),
                  Tab(icon: Icon(Icons.history_toggle_off, size: 16), text: '4D Flight Recorder'),
                  Tab(icon: Icon(Icons.lens_blur, size: 16), text: 'Epistemic Manifold'),
                  Tab(icon: Icon(Icons.hub_outlined, size: 16), text: 'Post-QIR Circuits'),
                  Tab(icon: Icon(Icons.radar, size: 16), text: 'Fleet Radar (T0-T3)'),
                  Tab(icon: Icon(Icons.psychology, size: 16), text: 'Cognitive LLM Mesh'),
                  Tab(icon: Icon(Icons.menu_book, size: 16), text: 'Citable Results Studio'),
                ],
              ),
            ),
            Expanded(
              child: TabBarView(
                children: [
                  _AutonomyCockpitTab(
                    api: widget.api,
                    sessions: _sessions!,
                    approvals: _approvals!,
                    onChanged: _refresh,
                  ),
                  _FlightRecorderTab(
                    api: widget.api,
                    loops: _loops!,
                    onChanged: _refresh,
                  ),
                  _EpistemicManifoldTab(
                    reports: _reports!,
                  ),
                  _CircuitVisualizerTab(),
                  _FleetRadarTab(
                    fleet: _fleet!,
                  ),
                  _CognitiveMeshTab(
                    api: widget.api,
                    models: _models!,
                    settings: _settings!,
                    onChanged: _refresh,
                  ),
                  _ResultsStudioTab(
                    reports: _reports!,
                    provenance: _provenance!,
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// 1. SURFACE 1: AUTONOMY COCKPIT & CRYPTOGRAPHIC AP2 SPEND SURFACE
// ---------------------------------------------------------------------------
class _AutonomyCockpitTab extends StatelessWidget {
  final ConsoleApi api;
  final Future<List<SessionSummary>> sessions;
  final Future<List<ApprovalItem>> approvals;
  final VoidCallback onChanged;

  const _AutonomyCockpitTab({
    required this.api,
    required this.sessions,
    required this.approvals,
    required this.onChanged,
  });

  @override
  Widget build(BuildContext context) {
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        Row(
          children: [
            const Icon(Icons.shield_outlined, color: ObservatoryTheme.cyanCoherence, size: 18),
            const SizedBox(width: 8),
            Text(
              'ACTIVE RESEARCH SESSIONS & SPEND CEILINGS',
              style: TextStyle(
                fontSize: 12,
                fontWeight: FontWeight.bold,
                letterSpacing: 1.0,
                color: Colors.white.withOpacity(0.85),
              ),
            ),
          ],
        ),
        const SizedBox(height: 12),
        FutureBuilder<List<SessionSummary>>(
          future: sessions,
          builder: (context, snap) {
            if (snap.hasError) return Text('error: ${snap.error}');
            final list = snap.data ?? const [];
            if (list.isEmpty) {
              return const Card(
                child: Padding(
                  padding: EdgeInsets.all(16),
                  child: Text('No active tenant research sessions registered.'),
                ),
              );
            }
            return Column(children: [for (final s in list) _SessionTelemetryCard(session: s)]);
          },
        ),
        const SizedBox(height: 24),
        Row(
          children: [
            const Icon(Icons.key, color: ObservatoryTheme.electricViolet, size: 18),
            const SizedBox(width: 8),
            Text(
              'CRYPTOGRAPHIC AP2 SPEND MANDATE AUTHORIZATIONS',
              style: TextStyle(
                fontSize: 12,
                fontWeight: FontWeight.bold,
                letterSpacing: 1.0,
                color: Colors.white.withOpacity(0.85),
              ),
            ),
          ],
        ),
        const SizedBox(height: 12),
        FutureBuilder<List<ApprovalItem>>(
          future: approvals,
          builder: (context, snap) {
            final list = snap.data ?? const [];
            if (list.isEmpty) {
              return const Card(
                child: Padding(
                  padding: EdgeInsets.all(16),
                  child: Text('All autonomy spend mandates verified. No pending reviews.'),
                ),
              );
            }
            return Column(
              children: [
                for (final a in list)
                  _MandateAuthorizationCard(api: api, approval: a, onChanged: onChanged),
              ],
            );
          },
        ),
      ],
    );
  }
}

class _SessionTelemetryCard extends StatelessWidget {
  final SessionSummary session;
  const _SessionTelemetryCard({required this.session});

  @override
  Widget build(BuildContext context) {
    final pctSpent = session.ceilingUsd != null && session.ceilingUsd! > 0
        ? (session.spentUsd / session.ceilingUsd!).clamp(0.0, 1.0)
        : 0.0;

    return Card(
      margin: const EdgeInsets.symmetric(vertical: 6),
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Text(
                  session.name,
                  style: const TextStyle(fontWeight: FontWeight.bold, fontSize: 14),
                ),
                const SizedBox(width: 8),
                Container(
                  padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
                  decoration: BoxDecoration(
                    color: ObservatoryTheme.cyanCoherence.withOpacity(0.12),
                    borderRadius: BorderRadius.circular(4),
                  ),
                  child: Text(
                    'owner: ${session.owner}',
                    style: const TextStyle(fontSize: 11, color: ObservatoryTheme.cyanCoherence),
                  ),
                ),
                const Spacer(),
                Text(
                  '\$${session.spentUsd.toStringAsFixed(2)} / \$${session.ceilingUsd?.toStringAsFixed(2) ?? 'unlimited'}',
                  style: const TextStyle(
                      fontFamily: 'monospace', fontWeight: FontWeight.bold, fontSize: 13),
                ),
              ],
            ),
            const SizedBox(height: 12),
            ClipRRect(
              borderRadius: BorderRadius.circular(4),
              child: LinearProgressIndicator(
                value: pctSpent,
                backgroundColor: Colors.white12,
                color: pctSpent > 0.8
                    ? ObservatoryTheme.contestedCrimson
                    : ObservatoryTheme.cyanCoherence,
                minHeight: 6,
              ),
            ),
            const SizedBox(height: 8),
            Row(
              children: [
                Text(
                  'Burn rate: \$${(session.burnRateUsdPerSec * 3600).toStringAsFixed(3)}/hr',
                  style: TextStyle(fontSize: 11, color: Colors.white.withOpacity(0.6)),
                ),
                const Spacer(),
                Text(
                  'Jobs: ${session.jobs.length}',
                  style: TextStyle(fontSize: 11, color: Colors.white.withOpacity(0.6)),
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }
}

class _MandateAuthorizationCard extends StatelessWidget {
  final ConsoleApi api;
  final ApprovalItem approval;
  final VoidCallback onChanged;

  const _MandateAuthorizationCard({
    required this.api,
    required this.approval,
    required this.onChanged,
  });

  @override
  Widget build(BuildContext context) {
    final isPending = approval.state == 'REVIEW' || approval.state == 'HOLD';

    return Card(
      margin: const EdgeInsets.symmetric(vertical: 6),
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(
                  Icons.vpn_key_outlined,
                  color: isPending
                      ? ObservatoryTheme.unverifiedAmber
                      : ObservatoryTheme.corroboratedGreen,
                  size: 20,
                ),
                const SizedBox(width: 10),
                Expanded(
                  child: Text(
                    approval.reason,
                    style: const TextStyle(fontWeight: FontWeight.bold, fontSize: 13),
                  ),
                ),
                Text(
                  '\$${approval.amountUsd.toStringAsFixed(2)}',
                  style: const TextStyle(
                    fontFamily: 'monospace',
                    fontWeight: FontWeight.bold,
                    fontSize: 14,
                    color: ObservatoryTheme.cyanCoherence,
                  ),
                ),
              ],
            ),
            const SizedBox(height: 8),
            Row(
              children: [
                Container(
                  padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
                  decoration: BoxDecoration(
                    color: ObservatoryTheme.electricViolet.withOpacity(0.15),
                    borderRadius: BorderRadius.circular(4),
                  ),
                  child: const Text('AP2 Mandate Protocol',
                      style: TextStyle(fontSize: 10, color: ObservatoryTheme.electricViolet)),
                ),
                if (approval.mandateDigest != null) ...[
                  const SizedBox(width: 8),
                  Text('digest: ${approval.mandateDigest!.substring(0, 8)}…',
                      style: const TextStyle(fontSize: 10, fontFamily: 'monospace')),
                ],
                const Spacer(),
                Container(
                  padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
                  decoration: BoxDecoration(
                    color: isPending
                        ? ObservatoryTheme.unverifiedAmber.withOpacity(0.15)
                        : ObservatoryTheme.corroboratedGreen.withOpacity(0.15),
                    borderRadius: BorderRadius.circular(4),
                  ),
                  child: Text(
                    approval.state,
                    style: TextStyle(
                      fontSize: 10,
                      fontWeight: FontWeight.bold,
                      color: isPending
                          ? ObservatoryTheme.unverifiedAmber
                          : ObservatoryTheme.corroboratedGreen,
                    ),
                  ),
                ),
              ],
            ),
            if (isPending) ...[
              const SizedBox(height: 12),
              Row(
                mainAxisAlignment: MainAxisAlignment.end,
                children: [
                  OutlinedButton(
                    onPressed: () async {
                      await api.approvalAction(approval.approvalId, 'hold');
                      onChanged();
                    },
                    child: const Text('Hold'),
                  ),
                  const SizedBox(width: 8),
                  OutlinedButton(
                    style: OutlinedButton.styleFrom(
                      foregroundColor: ObservatoryTheme.contestedCrimson,
                      side: const BorderSide(color: ObservatoryTheme.contestedCrimson),
                    ),
                    onPressed: () async {
                      await api.approvalAction(approval.approvalId, 'reject');
                      onChanged();
                    },
                    child: const Text('Deny'),
                  ),
                  const SizedBox(width: 8),
                  ElevatedButton.icon(
                    icon: const Icon(Icons.check, size: 14),
                    label: const Text('Authorize (Sign AP2)'),
                    style: ElevatedButton.styleFrom(
                      backgroundColor: ObservatoryTheme.cyanCoherence,
                      foregroundColor: Colors.black,
                    ),
                    onPressed: () async {
                      await api.approvalAction(approval.approvalId, 'approve');
                      onChanged();
                    },
                  ),
                ],
              ),
            ],
          ],
        ),
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// 2. SURFACE 2: 4D FLIGHT RECORDER & SCRUBBABLE LOOP INSPECTOR
// ---------------------------------------------------------------------------
class _FlightRecorderTab extends StatelessWidget {
  final ConsoleApi api;
  final Future<List<LoopItem>> loops;
  final VoidCallback onChanged;

  const _FlightRecorderTab({
    required this.api,
    required this.loops,
    required this.onChanged,
  });

  @override
  Widget build(BuildContext context) {
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        Row(
          children: [
            const Icon(Icons.history_toggle_off, color: ObservatoryTheme.cyanCoherence, size: 18),
            const SizedBox(width: 8),
            Text(
              '4D FLIGHT RECORDER & RING 0 WATCHDOG INSPECTOR',
              style: TextStyle(
                fontSize: 12,
                fontWeight: FontWeight.bold,
                letterSpacing: 1.0,
                color: Colors.white.withOpacity(0.85),
              ),
            ),
          ],
        ),
        const SizedBox(height: 12),
        FutureBuilder<List<LoopItem>>(
          future: loops,
          builder: (context, snap) {
            if (snap.hasError) return Text('error: ${snap.error}');
            final list = snap.data ?? const [];
            if (list.isEmpty) {
              return const Card(
                child: Padding(
                  padding: EdgeInsets.all(16),
                  child: Text('No autonomous loops currently registered.'),
                ),
              );
            }
            return Column(
              children: [
                for (final loop in list)
                  Padding(
                    padding: const EdgeInsets.only(bottom: 16),
                    child: FlightRecorderWidget(
                      api: api,
                      loopId: loop.loopId,
                      loopName: loop.name,
                      status: loop.status,
                      onChanged: onChanged,
                    ),
                  ),
              ],
            );
          },
        ),
      ],
    );
  }
}

// ---------------------------------------------------------------------------
// 3. SURFACE 3: EPISTEMIC MANIFOLD & CLASSICAL ANCHOR HORIZON
// ---------------------------------------------------------------------------
class _EpistemicManifoldTab extends StatelessWidget {
  final Future<List<Map<String, dynamic>>> reports;
  const _EpistemicManifoldTab({required this.reports});

  @override
  Widget build(BuildContext context) {
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        Row(
          children: [
            const Icon(Icons.lens_blur, color: ObservatoryTheme.cyanCoherence, size: 18),
            const SizedBox(width: 8),
            Text(
              'TRIPARTITE WAVEFUNCTIONS & CLASSICAL PHYSICS ANCHORS',
              style: TextStyle(
                fontSize: 12,
                fontWeight: FontWeight.bold,
                letterSpacing: 1.0,
                color: Colors.white.withOpacity(0.85),
              ),
            ),
          ],
        ),
        const SizedBox(height: 16),
        FutureBuilder<List<Map<String, dynamic>>>(
          future: reports,
          builder: (context, snap) {
            final list = snap.data ?? const [];
            if (list.isEmpty) {
              return const Card(
                child: Padding(
                  padding: EdgeInsets.all(16),
                  child: Text('No epistemic campaign claims generated yet.'),
                ),
              );
            }

            final latest = list.last;
            final claims = (latest['claims'] as List? ?? []).cast<Map<String, dynamic>>();
            final anchor = latest['classical_anchor'] as Map<String, dynamic>?;

            final firstClaimState = claims.isNotEmpty
                ? claims.first['epistemic_state'] as Map<String, dynamic>?
                : null;
            final probs = firstClaimState?['probabilities'] as Map<String, dynamic>? ?? {};

            return Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Card(
                  child: Padding(
                    padding: const EdgeInsets.all(16),
                    child: Row(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        WavefunctionRingGauge(
                          pCorroborated: (probs['corroborated'] as num?)?.toDouble() ?? 0.85,
                          pContested: (probs['contested'] as num?)?.toDouble() ?? 0.05,
                          pUnverified: (probs['unverified'] as num?)?.toDouble() ?? 0.10,
                          collapsed: firstClaimState?['collapsed'] as bool? ?? true,
                          verdict: firstClaimState?['verdict'] as String? ?? 'corroborated',
                        ),
                        const SizedBox(width: 20),
                        Expanded(
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              Text(
                                latest['title'] ?? 'Experiment Report',
                                style: const TextStyle(fontWeight: FontWeight.bold, fontSize: 15),
                              ),
                              const SizedBox(height: 6),
                              Text(
                                claims.isNotEmpty ? claims.first['claim'] : latest['conclusions'],
                                style: TextStyle(
                                  fontSize: 12,
                                  color: Colors.white.withOpacity(0.85),
                                  fontFamily: 'monospace',
                                ),
                              ),
                              const SizedBox(height: 12),
                              Row(
                                children: [
                                  _Badge(
                                    label: 'Ring 1 Gate',
                                    value: 'PASSED',
                                    color: ObservatoryTheme.corroboratedGreen,
                                  ),
                                  const SizedBox(width: 8),
                                  _Badge(
                                    label: 'Diversity',
                                    value:
                                        '${firstClaimState?['diversity_score'] ?? '0.60'}',
                                    color: ObservatoryTheme.cyanCoherence,
                                  ),
                                  const SizedBox(width: 8),
                                  _Badge(
                                    label: 'Entropy',
                                    value: '${firstClaimState?['entropy'] ?? '0.72'} bit',
                                    color: ObservatoryTheme.electricViolet,
                                  ),
                                ],
                              ),
                            ],
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
                const SizedBox(height: 16),
                if (anchor != null) ...[
                  Card(
                    child: Padding(
                      padding: const EdgeInsets.all(16),
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Row(
                            children: [
                              const Icon(Icons.balance,
                                  color: ObservatoryTheme.electricViolet, size: 16),
                              const SizedBox(width: 8),
                              Text(
                                'CLASSICAL PHYSICS ANCHOR: ${anchor['system_name']}',
                                style: const TextStyle(
                                    fontWeight: FontWeight.bold,
                                    fontSize: 12,
                                    color: ObservatoryTheme.electricViolet),
                              ),
                              const Spacer(),
                              const Text('Quantum Advantage Realism Enforced',
                                  style: TextStyle(
                                      fontSize: 11, color: ObservatoryTheme.cyanCoherence)),
                            ],
                          ),
                          const SizedBox(height: 12),
                          Row(
                            mainAxisAlignment: MainAxisAlignment.spaceAround,
                            children: [
                              _MetricCol(
                                label: 'RHF Baseline',
                                value: '${anchor['rhf_energy_hartree']} Ha',
                              ),
                              _MetricCol(
                                label: 'FCI Exact Ground',
                                value: '${anchor['fci_energy_hartree']} Ha',
                              ),
                              _MetricCol(
                                label: 'Correlation Energy',
                                value: '${anchor['correlation_energy_hartree']} Ha',
                              ),
                              _MetricCol(
                                label: 'Method',
                                value: '${anchor['classical_method']}',
                              ),
                            ],
                          ),
                        ],
                      ),
                    ),
                  ),
                ],
              ],
            );
          },
        ),
      ],
    );
  }
}

// ---------------------------------------------------------------------------
// 4. SURFACE 4: POST-QIR UNIVERSAL CIRCUIT VISUALIZER
// ---------------------------------------------------------------------------
class _CircuitVisualizerTab extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    return ListView(
      padding: const EdgeInsets.all(16),
      children: const [
        PostQirCircuitCanvas(
          numQubits: 2,
          title: 'Ansatz Trial: Variational 2-Qubit Entangler (RY-RY-CX)',
          gates: [
            {'name': 'ry', 'qubits': [0]},
            {'name': 'ry', 'qubits': [1]},
            {'name': 'cx', 'qubits': [0, 1]},
            {'name': 'meas', 'qubits': [0]},
            {'name': 'meas', 'qubits': [1]},
          ],
        ),
        SizedBox(height: 16),
        PostQirCircuitCanvas(
          numQubits: 3,
          title: 'ZNE Noise Amplification Circuit (Scale = 3)',
          gates: [
            {'name': 'x', 'qubits': [0]},
            {'name': 'cx', 'qubits': [0, 1]},
            {'name': 'cx', 'qubits': [0, 1]},
            {'name': 'cx', 'qubits': [0, 1]},
            {'name': 'meas', 'qubits': [0]},
            {'name': 'meas', 'qubits': [1]},
            {'name': 'meas', 'qubits': [2]},
          ],
        ),
      ],
    );
  }
}

// ---------------------------------------------------------------------------
// 5. SURFACE 5: EXECUTION FLEET RADAR
// ---------------------------------------------------------------------------
class _FleetRadarTab extends StatelessWidget {
  final Future<Map<String, dynamic>> fleet;
  const _FleetRadarTab({required this.fleet});

  @override
  Widget build(BuildContext context) {
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        Row(
          children: [
            const Icon(Icons.radar, color: ObservatoryTheme.cyanCoherence, size: 18),
            const SizedBox(width: 8),
            Text(
              '4-TIER HARDWARE CONTINUUM & JURISDICTION COMPLIANCE',
              style: TextStyle(
                fontSize: 12,
                fontWeight: FontWeight.bold,
                letterSpacing: 1.0,
                color: Colors.white.withOpacity(0.85),
              ),
            ),
          ],
        ),
        const SizedBox(height: 16),
        FutureBuilder<Map<String, dynamic>>(
          future: fleet,
          builder: (context, snap) {
            final reg = (snap.data?['fleet'] as Map<String, dynamic>?) ?? {};
            if (reg.isEmpty) {
              return const Card(
                child: Padding(
                  padding: EdgeInsets.all(16),
                  child: Text('Scanning fleet registry...'),
                ),
              );
            }

            return Column(
              children: [
                for (final entry in reg.entries)
                  Card(
                    margin: const EdgeInsets.symmetric(vertical: 6),
                    child: ListTile(
                      leading: Icon(
                        entry.value['canary'] == true
                            ? Icons.check_circle_outline
                            : Icons.cloud_queue,
                        color: entry.value['canary'] == true
                            ? ObservatoryTheme.corroboratedGreen
                            : ObservatoryTheme.cyanDim,
                      ),
                      title: Text(entry.key,
                          style: const TextStyle(fontWeight: FontWeight.bold, fontSize: 13)),
                      subtitle: Text(
                        'Modality: ${entry.value['modality']} · Pricing: ${entry.value['pricing']} · Status: ${entry.value['status']}',
                        style: TextStyle(fontSize: 11, color: Colors.white.withOpacity(0.7)),
                      ),
                      trailing: Container(
                        padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
                        decoration: BoxDecoration(
                          color: entry.value['canary'] == true
                              ? ObservatoryTheme.corroboratedGreen.withOpacity(0.15)
                              : Colors.white10,
                          borderRadius: BorderRadius.circular(4),
                        ),
                        child: Text(
                          entry.value['canary'] == true ? 'READY' : 'GATED',
                          style: TextStyle(
                            fontSize: 10,
                            fontWeight: FontWeight.bold,
                            color: entry.value['canary'] == true
                                ? ObservatoryTheme.corroboratedGreen
                                : Colors.white60,
                          ),
                        ),
                      ),
                    ),
                  ),
              ],
            );
          },
        ),
      ],
    );
  }
}

// ---------------------------------------------------------------------------
// 6. SURFACE 6: COGNITIVE LLM MESH & SOVEREIGN CONTROL DECK
// ---------------------------------------------------------------------------
class _CognitiveMeshTab extends StatefulWidget {
  final ConsoleApi api;
  final Future<Map<String, dynamic>> models;
  final Future<Map<String, dynamic>> settings;
  final VoidCallback onChanged;

  const _CognitiveMeshTab({
    required this.api,
    required this.models,
    required this.settings,
    required this.onChanged,
  });

  @override
  State<_CognitiveMeshTab> createState() => _CognitiveMeshTabState();
}

class _CognitiveMeshTabState extends State<_CognitiveMeshTab> {
  double _temperature = 0.2;
  String? _selectedModel = 'Qwen3.8-27B-Uncensored-OrcaRouter-MLX-8bit';

  @override
  Widget build(BuildContext context) {
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        Row(
          children: [
            const Icon(Icons.psychology, color: ObservatoryTheme.cyanCoherence, size: 18),
            const SizedBox(width: 8),
            Text(
              'COGNITIVE LLM MESH (OMLX PORT 8787 ACTIVE)',
              style: TextStyle(
                fontSize: 12,
                fontWeight: FontWeight.bold,
                letterSpacing: 1.0,
                color: Colors.white.withOpacity(0.85),
              ),
            ),
          ],
        ),
        const SizedBox(height: 16),
        Card(
          child: Padding(
            padding: const EdgeInsets.all(16),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    const Icon(Icons.memory, color: ObservatoryTheme.cyanCoherence),
                    const SizedBox(width: 10),
                    const Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text('Host OMLX Inference Engine',
                              style: TextStyle(fontWeight: FontWeight.bold, fontSize: 13)),
                          Text('http://127.0.0.1:8787 (OpenAI-compatible)',
                              style: TextStyle(fontSize: 11, color: Colors.white60)),
                        ],
                      ),
                    ),
                    Container(
                      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
                      decoration: BoxDecoration(
                        color: ObservatoryTheme.corroboratedGreen.withOpacity(0.15),
                        borderRadius: BorderRadius.circular(4),
                      ),
                      child: const Text('ONLINE',
                          style: TextStyle(
                              fontSize: 10,
                              fontWeight: FontWeight.bold,
                              color: ObservatoryTheme.corroboratedGreen)),
                    ),
                  ],
                ),
                const SizedBox(height: 16),
                const Text('Active Target Model', style: TextStyle(fontSize: 11, color: Colors.white70)),
                const SizedBox(height: 6),
                DropdownButtonFormField<String>(
                  value: _selectedModel,
                  dropdownColor: ObservatoryTheme.surface2,
                  decoration: InputDecoration(
                    contentPadding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
                    border: OutlineInputBorder(
                      borderRadius: BorderRadius.circular(6),
                      borderSide: const BorderSide(color: ObservatoryTheme.borderSubtle),
                    ),
                  ),
                  items: const [
                    DropdownMenuItem(
                      value: 'Qwen3.8-27B-Uncensored-OrcaRouter-MLX-8bit',
                      child: Text('Qwen3.8-27B-Uncensored-OrcaRouter-MLX-8bit (Primary)'),
                    ),
                    DropdownMenuItem(
                      value: 'Huihui-Qwen3.6-35B-A3B-Claude-4.7-Opus-abliterated-mlx-8bit',
                      child: Text('Huihui-Qwen3.6-35B-A3B-Claude-4.7-Opus'),
                    ),
                    DropdownMenuItem(
                      value: 'gemma-4-12b-coder-fable5-composer2.5-4bit',
                      child: Text('gemma-4-12b-coder'),
                    ),
                  ],
                  onChanged: (val) {
                    setState(() {
                      _selectedModel = val;
                    });
                  },
                ),
                const SizedBox(height: 16),
                Row(
                  children: [
                    const Text('Generation Temperature: ', style: TextStyle(fontSize: 11)),
                    Text(_temperature.toStringAsFixed(2),
                        style: const TextStyle(
                            fontFamily: 'monospace',
                            fontWeight: FontWeight.bold,
                            color: ObservatoryTheme.cyanCoherence)),
                    Expanded(
                      child: Slider(
                        value: _temperature,
                        min: 0.0,
                        max: 1.0,
                        activeColor: ObservatoryTheme.cyanCoherence,
                        onChanged: (val) {
                          setState(() {
                            _temperature = val;
                          });
                        },
                      ),
                    ),
                  ],
                ),
                const SizedBox(height: 8),
                Align(
                  alignment: Alignment.centerRight,
                  child: ElevatedButton(
                    style: ElevatedButton.styleFrom(
                      backgroundColor: ObservatoryTheme.cyanCoherence,
                      foregroundColor: Colors.black,
                    ),
                    onPressed: () async {
                      await widget.api.settingsPut({
                        'llm_provider': 'omlx',
                        'llm_base_url': 'http://127.0.0.1:8787',
                        'llm_model': _selectedModel,
                        'llm_temperature': _temperature,
                      });
                      widget.onChanged();
                      if (context.mounted) {
                        ScaffoldMessenger.of(context).showSnackBar(
                          const SnackBar(content: Text('Cognitive route updated to OMLX.')),
                        );
                      }
                    },
                    child: const Text('Bind Model Route'),
                  ),
                ),
              ],
            ),
          ),
        ),
      ],
    );
  }
}

// ---------------------------------------------------------------------------
// 7. SURFACE 7: CITABLE RESULTS STUDIO
// ---------------------------------------------------------------------------
class _ResultsStudioTab extends StatelessWidget {
  final Future<List<Map<String, dynamic>>> reports;
  final Future<Map<String, dynamic>> provenance;

  const _ResultsStudioTab({required this.reports, required this.provenance});

  @override
  Widget build(BuildContext context) {
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        Row(
          children: [
            const Icon(Icons.menu_book, color: ObservatoryTheme.cyanCoherence, size: 18),
            const SizedBox(width: 8),
            Text(
              'CITABLE RESULTS NOTEBOOK & PROVENANCE LEDGER',
              style: TextStyle(
                fontSize: 12,
                fontWeight: FontWeight.bold,
                letterSpacing: 1.0,
                color: Colors.white.withOpacity(0.85),
              ),
            ),
          ],
        ),
        const SizedBox(height: 16),
        FutureBuilder<List<Map<String, dynamic>>>(
          future: reports,
          builder: (context, snap) {
            final list = snap.data ?? const [];
            if (list.isEmpty) {
              return const Card(
                child: Padding(
                  padding: EdgeInsets.all(16),
                  child: Text('No completed experiment reports saved yet.'),
                ),
              );
            }
            final r = list.last;
            return Card(
              child: Padding(
                padding: const EdgeInsets.all(16),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      r['title'] ?? 'Experiment Report',
                      style: const TextStyle(fontWeight: FontWeight.bold, fontSize: 16),
                    ),
                    const SizedBox(height: 12),
                    _SectionBlock(title: 'OBJECTIVES', body: r['objectives'] ?? ''),
                    _SectionBlock(title: 'METHODS', body: r['methods'] ?? ''),
                    _SectionBlock(title: 'RESULTS', body: r['results'] ?? ''),
                    _SectionBlock(title: 'ANALYSIS', body: r['analysis'] ?? ''),
                    _SectionBlock(title: 'CONCLUSIONS', body: r['conclusions'] ?? ''),
                    const SizedBox(height: 12),
                    Text('CITATIONS',
                        style: TextStyle(
                            fontSize: 10,
                            fontWeight: FontWeight.bold,
                            color: Colors.white.withOpacity(0.6))),
                    const SizedBox(height: 4),
                    Text((r['citations'] as List? ?? []).join('; '),
                        style: const TextStyle(fontSize: 11, fontFamily: 'monospace')),
                  ],
                ),
              ),
            );
          },
        ),
      ],
    );
  }
}

class _SectionBlock extends StatelessWidget {
  final String title;
  final String body;
  const _SectionBlock({required this.title, required this.body});

  @override
  Widget build(BuildContext context) {
    if (body.isEmpty) return const SizedBox.shrink();
    return Padding(
      padding: const EdgeInsets.only(bottom: 10),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(title,
              style: TextStyle(
                  fontSize: 10,
                  fontWeight: FontWeight.bold,
                  letterSpacing: 0.8,
                  color: Colors.white.withOpacity(0.5))),
          const SizedBox(height: 2),
          Text(body, style: const TextStyle(fontSize: 12)),
        ],
      ),
    );
  }
}

class _Badge extends StatelessWidget {
  final String label;
  final String value;
  final Color color;

  const _Badge({required this.label, required this.value, required this.color});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
      decoration: BoxDecoration(
        color: color.withOpacity(0.12),
        borderRadius: BorderRadius.circular(4),
        border: Border.all(color: color.withOpacity(0.3)),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Text('$label: ', style: TextStyle(fontSize: 10, color: Colors.white.withOpacity(0.7))),
          Text(value, style: TextStyle(fontSize: 10, fontWeight: FontWeight.bold, color: color)),
        ],
      ),
    );
  }
}

class _MetricCol extends StatelessWidget {
  final String label;
  final String value;
  const _MetricCol({required this.label, required this.value});

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        Text(label, style: TextStyle(fontSize: 10, color: Colors.white.withOpacity(0.6))),
        const SizedBox(height: 2),
        Text(value,
            style: const TextStyle(
                fontSize: 12, fontWeight: FontWeight.bold, fontFamily: 'monospace')),
      ],
    );
  }
}
