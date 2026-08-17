/// Kubona console — control-plane client models + MCP gateway client.
///
/// Phase 2 delivers the two priority surfaces (cost & approval console, circuit
/// visualizer) as data contracts + a JSON-RPC client over the gateway's
/// streamable-HTTP transport. Widgets are built on a Flutter-equipped machine;
/// these models mirror the MCP tool payloads exactly (single source: the
/// platform's MCP servers).
library kubona_console;

export 'src/models.dart';
export 'src/gateway_client.dart';
export 'src/surfaces.dart';
