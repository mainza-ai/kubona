/// Console REST client — talks to the read/approve-only console API.
/// (Job submission is deliberately NOT exposed here; see console_api.py.)

import 'dart:convert';
import 'package:http/http.dart' as http;

class SessionSummary {
  final String sessionId;
  final String name;
  final String owner;
  final double spentUsd;
  final double? ceilingUsd;
  final double? remainingUsd;
  final String status;
  final double burnRateUsdPerSec;
  final List<String> jobs;

  const SessionSummary({
    required this.sessionId,
    required this.name,
    required this.owner,
    required this.spentUsd,
    this.ceilingUsd,
    this.remainingUsd,
    required this.status,
    required this.burnRateUsdPerSec,
    required this.jobs,
  });

  factory SessionSummary.fromJson(Map<String, dynamic> j) => SessionSummary(
        sessionId: j['session_id'] as String,
        name: j['name'] as String,
        owner: j['owner'] as String,
        spentUsd: (j['spent_usd'] as num).toDouble(),
        ceilingUsd: (j['budget_ceiling_usd'] as num?)?.toDouble(),
        remainingUsd: (j['remaining_usd'] as num?)?.toDouble(),
        status: j['status'] as String,
        burnRateUsdPerSec: (j['burn_rate_usd_per_s'] as num).toDouble(),
        jobs: (j['jobs'] as List).cast<String>(),
      );

  bool get breached => ceilingUsd != null && spentUsd > ceilingUsd!;
}

class ApprovalItem {
  final String approvalId;
  final String sessionId;
  final String state;
  final double amountUsd;
  final String reason;

  const ApprovalItem({
    required this.approvalId,
    required this.sessionId,
    required this.state,
    required this.amountUsd,
    required this.reason,
  });

  factory ApprovalItem.fromJson(Map<String, dynamic> j) => ApprovalItem(
        approvalId: j['approval_id'] as String,
        sessionId: j['session_id'] as String,
        state: j['state'] as String,
        amountUsd: (j['amount_requested_usd'] as num).toDouble(),
        reason: j['reason'] as String,
      );
}

class LoopItem {
  final String loopId;
  final String name;
  final String status;
  final int step;
  final int totalSteps;
  final String currentStepDesc;

  const LoopItem({
    required this.loopId,
    required this.name,
    required this.status,
    required this.step,
    required this.totalSteps,
    required this.currentStepDesc,
  });

  factory LoopItem.fromJson(Map<String, dynamic> j) => LoopItem(
        loopId: j['loop_id'] as String,
        name: j['name'] as String,
        status: j['status'] as String,
        step: j['step'] as int,
        totalSteps: j['total_steps'] as int,
        currentStepDesc: j['current_step_desc'] as String,
      );
}

class ConsoleApi {
  final String baseUrl;
  final String token;
  final http.Client _client;

  ConsoleApi({String? baseUrl, String? token, http.Client? client})
      : baseUrl = baseUrl ?? const String.fromEnvironment('KUBONA_API',
            defaultValue: 'http://localhost:8000'),
        token = token ?? const String.fromEnvironment('KUBONA_TOKEN',
            defaultValue: 'dev-token'),
        _client = client ?? http.Client();

  Map<String, String> get _headers => {
        'Authorization': 'Bearer $token',
        'content-type': 'application/json',
      };

  Future<List<SessionSummary>> sessions() async {
    final res = await _client.get(Uri.parse('$baseUrl/api/sessions'), headers: _headers);
    final body = jsonDecode(res.body) as Map<String, dynamic>;
    return (body['sessions'] as List)
        .map((e) => SessionSummary.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  Future<List<ApprovalItem>> approvals() async {
    final res = await _client.get(Uri.parse('$baseUrl/api/approvals'), headers: _headers);
    final body = jsonDecode(res.body) as Map<String, dynamic>;
    return (body['approvals'] as List)
        .map((e) => ApprovalItem.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  Future<List<LoopItem>> loops() async {
    final res = await _client.get(Uri.parse('$baseUrl/api/loops'), headers: _headers);
    final body = jsonDecode(res.body) as Map<String, dynamic>;
    return (body['loops'] as List)
        .map((e) => LoopItem.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  Future<Map<String, dynamic>> provenance() async {
    final res = await _client.get(Uri.parse('$baseUrl/api/provenance'), headers: _headers);
    return jsonDecode(res.body) as Map<String, dynamic>;
  }

  Future<Map<String, dynamic>> fleet() async {
    final res = await _client.get(Uri.parse('$baseUrl/api/fleet'), headers: _headers);
    return jsonDecode(res.body) as Map<String, dynamic>;
  }

  Future<String> approvalAction(String id, String action) async {
    final res = await _client.post(Uri.parse('$baseUrl/api/approvals/$id/$action'),
        headers: _headers);
    final body = jsonDecode(res.body) as Map<String, dynamic>;
    if (body['ok'] != true) {
      throw Exception(body['error']);
    }
    return body['state'] as String;
  }

  Future<String> loopAction(String id, String action) async {
    final res = await _client.post(Uri.parse('$baseUrl/api/loops/$id/$action'),
        headers: _headers);
    final body = jsonDecode(res.body) as Map<String, dynamic>;
    if (body['ok'] != true) {
      throw Exception(body['error']);
    }
    return body['status'] as String;
  }
}
