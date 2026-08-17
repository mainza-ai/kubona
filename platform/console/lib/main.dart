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
        length: 2,
        child: Column(
          children: [
            const TabBar(tabs: [Tab(text: 'Cost & Approvals'), Tab(text: 'Live Loops')]),
            Expanded(
              child: TabBarView(
                children: [
                  _CostApprovalTab(api: widget.api, sessions: _sessions!, approvals: _approvals!,
                      onChanged: _refresh),
                  _LoopsTab(api: widget.api, loops: _loops!, onChanged: _refresh),
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
