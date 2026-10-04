import 'package:flutter/material.dart';
import '../api_client.dart';
import '../theme/observatory_theme.dart';

/// 4D Flight Recorder & Interactive Scrubbable Loop Inspector.
/// Allows time-travel scrubbing, telemetry inspection, and Ring 0 watchdog tracking.
class FlightRecorderWidget extends StatefulWidget {
  final ConsoleApi api;
  final String loopId;
  final String loopName;
  final String status;
  final VoidCallback onChanged;

  const FlightRecorderWidget({
    super.key,
    required this.api,
    required this.loopId,
    required this.loopName,
    required this.status,
    required this.onChanged,
  });

  @override
  State<FlightRecorderWidget> createState() => _FlightRecorderWidgetState();
}

class _FlightRecorderWidgetState extends State<FlightRecorderWidget> {
  int _scrubStep = 0;
  List<Map<String, dynamic>> _history = [];
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    _loadHistory();
  }

  Future<void> _loadHistory() async {
    try {
      final res = await widget.api.loopHistory(widget.loopId);
      final rawList = res['history'] as List? ?? [];
      setState(() {
        _history = rawList.cast<Map<String, dynamic>>();
        _scrubStep = _history.isNotEmpty ? _history.length : 0;
        _loading = false;
      });
    } catch (_) {
      setState(() {
        _loading = false;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) {
      return const Center(child: CircularProgressIndicator(color: ObservatoryTheme.cyanCoherence));
    }

    final activeStepData = (_history.isNotEmpty && _scrubStep > 0 && _scrubStep <= _history.length)
        ? _history[_scrubStep - 1]
        : null;

    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: ObservatoryTheme.surface1,
        borderRadius: BorderRadius.circular(10),
        border: Border.all(color: ObservatoryTheme.borderSubtle),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              const Icon(Icons.history_toggle_off, color: ObservatoryTheme.cyanCoherence),
              const SizedBox(width: 8),
              Expanded(
                child: Text(
                  '4D FLIGHT RECORDER: ${widget.loopName.toUpperCase()}',
                  style: const TextStyle(fontWeight: FontWeight.bold, fontSize: 13, letterSpacing: 0.5),
                  overflow: TextOverflow.ellipsis,
                ),
              ),
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
                decoration: BoxDecoration(
                  color: widget.status == 'running'
                      ? ObservatoryTheme.cyanCoherence.withOpacity(0.15)
                      : ObservatoryTheme.unverifiedAmber.withOpacity(0.15),
                  borderRadius: BorderRadius.circular(4),
                ),
                child: Text(
                  widget.status.toUpperCase(),
                  style: TextStyle(
                    fontSize: 10,
                    fontWeight: FontWeight.bold,
                    color: widget.status == 'running'
                        ? ObservatoryTheme.cyanCoherence
                        : ObservatoryTheme.unverifiedAmber,
                  ),
                ),
              ),
            ],
          ),
          const SizedBox(height: 12),
          // Time-travel scrubber slider
          if (_history.length > 1) ...[
            Row(
              children: [
                Text(
                  'Scrub Timeline (Step $_scrubStep / ${_history.length})',
                  style: TextStyle(fontSize: 11, color: Colors.white.withOpacity(0.7)),
                ),
                const Spacer(),
                Text(
                  activeStepData?['desc'] ?? '',
                  style: const TextStyle(fontSize: 11, fontFamily: 'monospace', color: ObservatoryTheme.cyanCoherence),
                ),
              ],
            ),
            Slider(
              value: _scrubStep.toDouble(),
              min: 1.0,
              max: _history.length.toDouble(),
              divisions: _history.length - 1,
              activeColor: ObservatoryTheme.cyanCoherence,
              inactiveColor: Colors.white12,
              onChanged: (val) {
                setState(() {
                  _scrubStep = val.toInt();
                });
              },
            ),
          ],
          if (activeStepData != null) ...[
            Container(
              padding: const EdgeInsets.all(12),
              decoration: BoxDecoration(
                color: ObservatoryTheme.surface2,
                borderRadius: BorderRadius.circular(6),
                border: Border.all(color: ObservatoryTheme.borderSubtle),
              ),
              child: Row(
                children: [
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        const Text('STATE TELEMETRY AT STEP', style: TextStyle(fontSize: 10, color: Colors.white54)),
                        const SizedBox(height: 4),
                        Text(
                          'Metric: ${activeStepData['metric'] ?? 'nominal'} · Param: ${activeStepData['params'] ?? 'ansatz_theta'}',
                          style: const TextStyle(fontSize: 12, fontFamily: 'monospace', color: Colors.white),
                        ),
                      ],
                    ),
                  ),
                  if (activeStepData['watchdog'] != null)
                    Container(
                      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
                      decoration: BoxDecoration(
                        color: ObservatoryTheme.contestedCrimson.withOpacity(0.15),
                        borderRadius: BorderRadius.circular(4),
                      ),
                      child: Text(
                        'Ring 0: ${activeStepData['watchdog']}',
                        style: const TextStyle(fontSize: 10, color: ObservatoryTheme.contestedCrimson),
                      ),
                    ),
                ],
              ),
            ),
          ],
          const SizedBox(height: 12),
          // Ring 0 Supervisory Override Controls
          Row(
            mainAxisAlignment: MainAxisAlignment.end,
            children: [
              OutlinedButton.icon(
                icon: const Icon(Icons.pause, size: 14),
                label: const Text('Pause'),
                style: OutlinedButton.styleFrom(
                  foregroundColor: Colors.white70,
                  side: const BorderSide(color: ObservatoryTheme.borderSubtle),
                ),
                onPressed: () async {
                  await widget.api.loopAction(widget.loopId, 'pause');
                  widget.onChanged();
                },
              ),
              const SizedBox(width: 8),
              OutlinedButton.icon(
                icon: const Icon(Icons.play_arrow, size: 14),
                label: const Text('Resume'),
                style: OutlinedButton.styleFrom(
                  foregroundColor: ObservatoryTheme.cyanCoherence,
                  side: const BorderSide(color: ObservatoryTheme.cyanDim),
                ),
                onPressed: () async {
                  await widget.api.loopAction(widget.loopId, 'resume');
                  widget.onChanged();
                },
              ),
              const SizedBox(width: 8),
              OutlinedButton.icon(
                icon: const Icon(Icons.stop, size: 14),
                label: const Text('Abort to Snapshot'),
                style: OutlinedButton.styleFrom(
                  foregroundColor: ObservatoryTheme.contestedCrimson,
                  side: const BorderSide(color: ObservatoryTheme.contestedCrimson),
                ),
                onPressed: () async {
                  await widget.api.loopAction(widget.loopId, 'kill');
                  widget.onChanged();
                },
              ),
            ],
          ),
        ],
      ),
    );
  }
}
