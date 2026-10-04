import 'dart:convert';

import 'package:flutter/material.dart';
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
    if (body.contains('/api/loops/') && body.endsWith('/history')) {
      return http.Response(jsonEncode({
        'ok': true,
        'loop_id': 'l1',
        'name': 'ansatz-search',
        'status': 'running',
        'step': 3,
        'total_steps': 100,
        'history': [
          {'step': 1, 'desc': 'init ansatz', 'metric': 0.5, 'params': '0.1'},
          {'step': 2, 'desc': 'gradient descent', 'metric': 0.8, 'params': '0.3'},
          {'step': 3, 'desc': 'measure energy', 'metric': 1.1, 'params': '0.5'},
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
    if (body.endsWith('/api/models')) {
      return http.Response(jsonEncode({
        'ok': true,
        'scan': {
          '8787': {'base_url': 'http://127.0.0.1:8787', 'port': 8787, 'provider': 'openai-compat',
                   'models': [{'model_id': 'Qwen3.6-35B-A3B-UD-MLX-4bit'},
                              {'model_id': 'Llama-3.2-3B-Instruct-bf16'}]},
          'summary': {'8787': 'openai-compat'},
        },
        'ports': [11434, 8787, 1234, 8080],
      }), 200, headers: {'content-type': 'application/json'});
    }
    if (body.endsWith('/api/settings')) {
      if (request.method == 'PUT') {
        return http.Response(jsonEncode({'ok': true, 'settings': {'llm_provider': 'omlx',
            'llm_model': 'Qwen3.6-35B-A3B-UD-MLX-4bit', 'llm_temperature': 0.1,
            'host_available': true}}), 200, headers: {'content-type': 'application/json'});
      }
      return http.Response(jsonEncode({'ok': true, 'settings': {'llm_provider': 'scripted',
          'llm_model': '', 'host_available': true}}), 200, headers: {'content-type': 'application/json'});
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
    tester.view.physicalSize = const Size(1920, 1080);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(() {
      tester.view.resetPhysicalSize();
      tester.view.resetDevicePixelRatio();
    });

    await tester.pumpWidget(KubonaConsole(api: _fakeApi()));
    await tester.pumpAndSettle();

    expect(find.text('KUBONA OBSERVATORY'), findsOneWidget);
    expect(find.textContaining('vqe-h2'), findsOneWidget);
    expect(find.textContaining('paid provider run'), findsOneWidget);

    // Tap 4D Flight Recorder (Tab 1)
    await tester.tap(find.byIcon(Icons.history_toggle_off));
    await tester.pumpAndSettle();
    expect(find.textContaining('ANSATZ-SEARCH'), findsOneWidget);

    // Tap Cognitive LLM Mesh (Tab 5)
    await tester.tap(find.byIcon(Icons.psychology));
    await tester.pumpAndSettle();
    expect(find.textContaining('OMLX'), findsWidgets);

    // Tap Epistemic Manifold (Tab 2)
    await tester.tap(find.byIcon(Icons.lens_blur));
    await tester.pumpAndSettle();
    expect(find.textContaining('TRIPARTITE WAVEFUNCTIONS'), findsOneWidget);
  });
}
