/// JSON-RPC 2.0 client for the Kubona MCP gateway (streamable HTTP).
///
/// The console talks ONLY to the gateway (read/approve surface); it never
/// submits jobs outside the MCP-governed flow. Auth: bearer token (OAuth 2.1
/// dev token today; Phase 3 hardens transport + PQC).

import 'dart:convert';
import 'package:http/http.dart' as http;

class GatewayClient {
  final Uri endpoint;
  final String token;
  final http.Client _client;

  GatewayClient(this.endpoint, {required this.token, http.Client? client})
      : _client = client ?? http.Client();

  Future<Map<String, dynamic>> call(
    String server,
    String tool,
    Map<String, dynamic> args,
  ) async {
    final body = jsonEncode({
      'jsonrpc': '2.0',
      'id': 1,
      'method': 'tools/call',
      'params': {
        'name': '$server/$tool',
        'arguments': args,
      },
    });
    final res = await _client.post(
      endpoint,
      headers: {
        'content-type': 'application/json',
        'authorization': 'Bearer $token',
      },
      body: body,
    );
    if (res.statusCode != 200) {
      throw GatewayException(res.statusCode, res.body);
    }
    final decoded = jsonDecode(res.body) as Map<String, dynamic>;
    if (decoded.containsKey('error')) {
      throw GatewayException(res.statusCode, decoded['error'].toString());
    }
    return decoded['result'] as Map<String, dynamic>;
  }
}

class GatewayException implements Exception {
  final int statusCode;
  final String detail;
  GatewayException(this.statusCode, this.detail);
  @override
  String toString() => 'gateway error $statusCode: $detail';
}
