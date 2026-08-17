import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

import 'package:kubona_console/main.dart';
import 'package:kubona_console/src/api_client.dart';

ConsoleApi _fakeApi() {
  final client = MockClient((request) async {
    final body = request.url.path;
    if (body.endsWith('/api/sessions')) {
      return http.Response(jsonEncode({
        'ok': true,
        'sessions': [
          {
            'session_id': 's1', 'name': 'vqe-h2', 'owner': 'researcher',
            'spent_usd': 3.5, 'budget_ceiling_usd': 50.0, 'remaining_usd': 46.5,
            'status': 'open', 'burn_rate_usd_per_s': 0.02, 'jobs': ['j1', 'j2'],
          }
        ],
      }), 200, headers: {'content-type': 'application/json'});
    }
    if (body.endsWith('/api/approvals')) {
      return http.Response(jsonEncode({
        'ok': true,
        'approvals': [
          {
            'approval_id': 'a1', 'session_id': 's1', 'state': 'REVIEW',
            'amount_requested_usd': 42.0, 'reason': 'paid provider run',
          }
        ],
      }), 200, headers: {'content-type': 'application/json'});
    }
    if (body.endsWith('/api/loops')) {
      return http.Response(jsonEncode({
        'ok': true,
        'loops': [
          {
            'loop_id': 'l1', 'name': 'ansatz-search', 'status': 'running',
            'step': 3, 'total_steps': 100, 'current_step_desc': 'measure energy',
          }
        ],
      }), 200, headers: {'content-type': 'application/json'});
    }
    if (body.endsWith('/api/reports')) {
      return http.Response(jsonEncode({
        'ok': true,
        'reports': [
          {
            'report_id': 'r1', 'title': 'Energetics of Z0Z1+0.5X0X1',
            'objectives': 'find ground state', 'methods': 'VQE search',
            'results': 'metric 1.1', 'analysis': 'ZNE recovered',
            'conclusions': 'loop works', 'refs': [['results', 'ledger:abc']],
          }
        ],
      }), 200, headers: {'content-type': 'application/json'});
    }
    if (body.endsWith('/api/workflows')) {
      return http.Response(jsonEncode({'ok': true, 'workflows': [
        {'workflow_id': 'w1', 'name': 'vqe-sweep', 'created_by': 'researcher', 'created_by_agent': false}
      ]}), 200, headers: {'content-type': 'application/json'});
    }
    if (body.endsWith('/api/agents')) {
      return http.Response(jsonEncode({'ok': true, 'agents': [
        {'agent_id': 'a1', 'persona': 'ansatz-specialist', 'scopes': ['execution:submit'], 'created_by': null}
      ]}), 200, headers: {'content-type': 'application/json'});
    }
    if (body.endsWith('/api/labs')) {
      return http.Response(jsonEncode({'ok': true, 'labs': [
        {'lab_id': 'l1', 'name': 'Alpha Lab', 'tenant': 'alpha-lab', 'owner': 'researcher', 'members': ['researcher', 'agent-1']}
      ]}), 200, headers: {'content-type': 'application/json'});
    }
    return http.Response('{"ok": true}', 200, headers: {'content-type': 'application/json'});
  });
  return ConsoleApi(baseUrl: 'http://fake', token: 'dev-token', client: client);
}

void main() {
  testWidgets('console renders sessions, approvals, and loops', (tester) async {
    await tester.pumpWidget(KubonaConsole(api: _fakeApi()));
    await tester.pumpAndSettle();

    expect(find.text('Kubona Console'), findsOneWidget);
    expect(find.textContaining('vqe-h2'), findsOneWidget);
    expect(find.textContaining('paid provider run'), findsOneWidget);

    await tester.tap(find.text('Live Loops'));
    await tester.pumpAndSettle();
    expect(find.textContaining('ansatz-search'), findsOneWidget);
    expect(find.textContaining('measure energy'), findsOneWidget);

    await tester.tap(find.text('Notebook'));
    await tester.pumpAndSettle();
    expect(find.textContaining('Energetics of Z0Z1+0.5X0X1'), findsOneWidget);

    await tester.tap(find.text('Labs & Workflows'));
    await tester.pumpAndSettle();
    expect(find.textContaining('Alpha Lab'), findsOneWidget);
    expect(find.textContaining('vqe-sweep'), findsOneWidget);
    expect(find.textContaining('ansatz-specialist'), findsOneWidget);
  });
}
