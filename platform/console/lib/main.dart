/// Kubona Console — Phase 2: cost & approval console + live loop inspector.
///
/// Read/approve only (ui-ux-vision hard rule #1): this app calls the console
/// REST API (console_api.py) and never submits jobs.

import 'package:flutter/material.dart';

import 'src/api_client.dart';

void main() {
  runApp(KubonaConsole(api: ConsoleApi()));
}

class KubonaConsole extends StatelessWidget {
  final ConsoleApi api;
  const KubonaConsole({super.key, required this.api});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Kubona Console',
      theme: ThemeData(
        colorScheme: ColorScheme.fromSeed(
            seedColor: const Color(0xFF1B4D89), brightness: Brightness.dark),
        useMaterial3: true,
      ),
      home: ConsoleHome(api: api),
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
    });
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Kubona Console'),
        actions: [
          IconButton(
            icon: const Icon(Icons.refresh),
            tooltip: 'Refresh',
            onPressed: _refresh,
          ),
        ],
      ),
      body: DefaultTabController(
        length: 6,
        child: Column(
          children: [
            const TabBar(tabs: [
              Tab(text: 'Cost & Approvals'),
              Tab(text: 'Live Loops'),
              Tab(text: 'Provenance'),
              Tab(text: 'Fleet'),
              Tab(text: 'Notebook'),
              Tab(text: 'Labs & Workflows'),
            ]),
            Expanded(
              child: TabBarView(
                children: [
                  _CostApprovalTab(api: widget.api, sessions: _sessions!, approvals: _approvals!,
                      onChanged: _refresh),
                  _LoopsTab(api: widget.api, loops: _loops!, onChanged: _refresh),
                  _ProvenanceTab(provenance: _provenance!),
                  _FleetTab(fleet: _fleet!),
                  _NotebookTab(reports: _reports!),
                  _LabsWorkflowsTab(labs: _labs!, workflows: _workflows!, agents: _agents!),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _CostApprovalTab extends StatelessWidget {
  final ConsoleApi api;
  final Future<List<SessionSummary>> sessions;
  final Future<List<ApprovalItem>> approvals;
  final VoidCallback onChanged;

  const _CostApprovalTab(
      {required this.api, required this.sessions, required this.approvals,
       required this.onChanged});

  @override
  Widget build(BuildContext context) {
    return ListView(padding: const EdgeInsets.all(12), children: [
      Text('Sessions & burn rate', style: Theme.of(context).textTheme.titleMedium),
      FutureBuilder<List<SessionSummary>>(
        future: sessions,
        builder: (context, snap) {
          if (snap.hasError) return Text('error: ${snap.error}');
          final list = snap.data ?? const [];
          return Column(children: [for (final s in list) _SessionCard(session: s)]);
        },
      ),
      const SizedBox(height: 16),
      Text('Pending approvals', style: Theme.of(context).textTheme.titleMedium),
      FutureBuilder<List<ApprovalItem>>(
        future: approvals,
        builder: (context, snap) {
          final list = (snap.data ?? const []).where((a) => a.state == 'REVIEW' || a.state == 'HOLD').toList();
          if (list.isEmpty) {
            return const Padding(padding: EdgeInsets.all(8), child: Text('No pending approvals.'));
          }
          return Column(children: [
            for (final a in list)
              Card(
                child: ListTile(
                  title: Text('${a.reason} — \$${a.amountUsd.toStringAsFixed(2)}'),
                  subtitle: Text('${a.sessionId} · ${a.state}'),
                  trailing: Row(mainAxisSize: MainAxisSize.min, children: [
                    IconButton(
                        icon: const Icon(Icons.check, color: Colors.green),
                        onPressed: () async {
                          await api.approvalAction(a.approvalId, 'approve');
                          onChanged();
                        }),
                    IconButton(
                        icon: const Icon(Icons.close, color: Colors.red),
                        onPressed: () async {
                          await api.approvalAction(a.approvalId, 'reject');
                          onChanged();
                        }),
                  ]),
                ),
              ),
          ]);
        },
      ),
    ]);
  }
}

class _ProvenanceTab extends StatelessWidget {
  final Future<Map<String, dynamic>> provenance;
  const _ProvenanceTab({required this.provenance});

  @override
  Widget build(BuildContext context) {
    return FutureBuilder<Map<String, dynamic>>(
      future: provenance,
      builder: (context, snap) {
        if (snap.hasError) return Text('error: ${snap.error}');
        final data = snap.data ?? const {};
        final recent = (data['recent'] as List? ?? const []);
        final agg = (data['aggregate'] as Map? ?? const {});
        return ListView(padding: const EdgeInsets.all(12), children: [
          Text('What worked best (per circuit hash)', style: Theme.of(context).textTheme.titleMedium),
          for (final entry in agg.entries.take(20))
            ListTile(
              dense: true,
              title: Text('${entry.key}'),
              subtitle: Text('${(entry.value['count'])} runs · '
                  'providers: ${(entry.value['providers'] as List).join(', ')}'),
            ),
          const SizedBox(height: 16),
          Text('Recent ledger records', style: Theme.of(context).textTheme.titleMedium),
          for (final r in recent.take(20).toList().reversed)
            ListTile(dense: true, title: Text(r.toString().length > 140 ? '${r.toString().substring(0, 140)}…' : r.toString())),
        ]);
      },
    );
  }
}

class _FleetTab extends StatelessWidget {
  final Future<Map<String, dynamic>> fleet;
  const _FleetTab({required this.fleet});

  @override
  Widget build(BuildContext context) {
    return FutureBuilder<Map<String, dynamic>>(
      future: fleet,
      builder: (context, snap) {
        if (snap.hasError) return Text('error: ${snap.error}');
        final data = snap.data ?? const {};
        final providers = (data['fleet'] as Map? ?? const {});
        return ListView(padding: const EdgeInsets.all(12), children: [
          for (final entry in providers.entries)
            Card(
              child: ListTile(
                title: Text(entry.key),
                subtitle: Text('${entry.value['modality']} · ${entry.value['status']} · '
                    '${entry.value['pricing']} · canary: ${entry.value['canary']}'),
                trailing: Icon(
                  entry.value['canary'] == true ? Icons.check_circle : Icons.warning,
                  color: entry.value['canary'] == true ? Colors.green : Colors.orange,
                ),
              ),
            ),
        ]);
      },
    );
  }
}

class _NotebookTab extends StatelessWidget {
  final Future<List<Map<String, dynamic>>> reports;
  const _NotebookTab({required this.reports});

  @override
  Widget build(BuildContext context) {
    return FutureBuilder<List<Map<String, dynamic>>>(
      future: reports,
      builder: (context, snap) {
        if (snap.hasError) return Text('error: ${snap.error}');
        final list = snap.data ?? const [];
        if (list.isEmpty) {
          return const Center(child: Text('No experiment notebooks yet. Run a discovery campaign.'));
        }
        return ListView(padding: const EdgeInsets.all(12), children: [
          for (final r in list.reversed)
            Card(
              child: ExpansionTile(
                title: Text(r['title'] ?? 'Report ${r['report_id']}'),
                subtitle: Text('conclusions: ${r['conclusions'] ?? ''}'.length > 0 ? '${(r['conclusions'] ?? '').toString().substring(0, 0)}refs: ${(r['refs'] as List).length}' : ''),
                children: [
                  for (final sec in ['objectives', 'methods', 'results', 'analysis'])
                    Padding(
                      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 4),
                      child: Text('${sec}: ${r[sec] ?? ''}'),
                    ),
                  Text('   claims traceable: ${((r['refs'] as List?)?.isNotEmpty ?? false)}',
                      style: const TextStyle(fontStyle: FontStyle.italic)),
                ],
              ),
            ),
        ]);
      },
    );
  }
}

class _LabsWorkflowsTab extends StatelessWidget {
  final Future<List<Map<String, dynamic>>> labs;
  final Future<List<Map<String, dynamic>>> workflows;
  final Future<List<Map<String, dynamic>>> agents;
  const _LabsWorkflowsTab(
      {required this.labs, required this.workflows, required this.agents});

  @override
  Widget build(BuildContext context) {
    return ListView(padding: const EdgeInsets.all(12), children: [
      Text('Labs', style: Theme.of(context).textTheme.titleMedium),
      FutureBuilder<List<Map<String, dynamic>>>(
        future: labs,
        builder: (context, snap) {
          final l = snap.data ?? const [];
          return Column(children: [
            for (final lab in l)
              ListTile(dense: true, title: Text('${lab['name']} (tenant ${lab['tenant']})'),
                       subtitle: Text('owner ${lab['owner']} · ${(lab['members'] as List).length} members')),
          ]);
        },
      ),
      Text('Workflows', style: Theme.of(context).textTheme.titleMedium),
      FutureBuilder<List<Map<String, dynamic>>>(
        future: workflows,
        builder: (context, snap) {
          final w = snap.data ?? const [];
          if (w.isEmpty) return const Padding(padding: EdgeInsets.all(8), child: Text('none saved'));
          return Column(children: [
            for (final wf in w)
              ListTile(dense: true,
                       title: Text(wf['name'] ?? ''),
                       subtitle: Text('by ${wf['created_by']} · agent-created: ${wf['created_by_agent']}')),
          ]);
        },
      ),
      Text('Agents', style: Theme.of(context).textTheme.titleMedium),
      FutureBuilder<List<Map<String, dynamic>>>(
        future: agents,
        builder: (context, snap) {
          final a = snap.data ?? const [];
          if (a.isEmpty) return const Padding(padding: EdgeInsets.all(8), child: Text('no agents'));
          return Column(children: [
            for (final ag in a)
              ListTile(dense: true,
                       title: Text('${ag['persona']} (${ag['agent_id']})'),
                       subtitle: Text('scopes: ${(ag['scopes'] as List).join(', ')} · created_by ${ag['created_by'] ?? 'owner'}')),
          ]);
        },
      ),
    ]);
  }
}

class _SessionCard extends StatelessWidget {
  final SessionSummary session;
  const _SessionCard({required this.session});

  @override
  Widget build(BuildContext context) {
    final color = session.breached ? Colors.red : Colors.green;
    return Card(
      child: ListTile(
        title: Text('${session.name} (${session.owner})'),
        subtitle: Text(
            'spent \$${session.spentUsd.toStringAsFixed(2)} / '
            '\$${(session.ceilingUsd ?? 0).toStringAsFixed(2)} · '
            'burn ${session.burnRateUsdPerSec.toStringAsFixed(3)}\$/s · '
            '${session.jobs.length} jobs'),
        trailing: Icon(Icons.circle, size: 14, color: color),
      ),
    );
  }
}

class _LoopsTab extends StatelessWidget {
  final ConsoleApi api;
  final Future<List<LoopItem>> loops;
  final VoidCallback onChanged;

  const _LoopsTab({required this.api, required this.loops, required this.onChanged});

  @override
  Widget build(BuildContext context) {
    return FutureBuilder<List<LoopItem>>(
      future: loops,
      builder: (context, snap) {
        if (snap.hasError) return Text('error: ${snap.error}');
        final list = snap.data ?? const [];
        if (list.isEmpty) {
          return const Center(child: Text('No loops running.'));
        }
        return ListView(padding: const EdgeInsets.all(12), children: [
          for (final l in list)
            Card(
              child: ListTile(
                title: Text('${l.name} — ${l.status}'),
                subtitle: Text('step ${l.step}/${l.totalSteps} · ${l.currentStepDesc}'),
                trailing: Row(mainAxisSize: MainAxisSize.min, children: [
                  if (l.status == 'running')
                    IconButton(
                        icon: const Icon(Icons.pause),
                        onPressed: () async {
                          await api.loopAction(l.loopId, 'pause');
                          onChanged();
                        }),
                  IconButton(
                      icon: const Icon(Icons.stop, color: Colors.red),
                      onPressed: () async {
                        await api.loopAction(l.loopId, 'kill');
                        onChanged();
                      }),
                ]),
              ),
            ),
        ]);
      },
    );
  }
}
