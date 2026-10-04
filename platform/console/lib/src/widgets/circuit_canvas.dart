import 'package:flutter/material.dart';
import '../theme/observatory_theme.dart';

/// Universal SDK-Independent Post-QIR Circuit Visualizer.
/// Renders horizontal qubit lines, quantum gate glyphs, and wire cuts.
class PostQirCircuitCanvas extends StatelessWidget {
  final int numQubits;
  final List<Map<String, dynamic>> gates;
  final String title;

  const PostQirCircuitCanvas({
    super.key,
    this.numQubits = 2,
    this.gates = const [
      {'name': 'h', 'qubits': [0]},
      {'name': 'cx', 'qubits': [0, 1]},
      {'name': 'ry', 'qubits': [0], 'param': '0.785'},
      {'name': 'meas', 'qubits': [0]},
      {'name': 'meas', 'qubits': [1]},
    ],
    this.title = 'Universal Post-QIR Lowered Topology',
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: ObservatoryTheme.surface2,
        borderRadius: BorderRadius.circular(10),
        border: Border.all(color: ObservatoryTheme.borderSubtle),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              const Icon(Icons.hub_outlined, color: ObservatoryTheme.cyanCoherence, size: 16),
              const SizedBox(width: 8),
              Text(
                title.toUpperCase(),
                style: const TextStyle(
                  fontSize: 11,
                  fontWeight: FontWeight.bold,
                  letterSpacing: 0.8,
                  color: ObservatoryTheme.cyanCoherence,
                ),
              ),
              const Spacer(),
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
                decoration: BoxDecoration(
                  color: ObservatoryTheme.electricViolet.withOpacity(0.15),
                  borderRadius: BorderRadius.circular(4),
                ),
                child: const Text('QIR / Native LLVM IR',
                    style: TextStyle(fontSize: 10, color: ObservatoryTheme.electricViolet)),
              ),
            ],
          ),
          const SizedBox(height: 16),
          SingleChildScrollView(
            scrollDirection: Axis.horizontal,
            child: CustomPaint(
              size: Size((gates.length * 55.0) + 80.0, numQubits * 44.0 + 20.0),
              painter: _CircuitPainter(numQubits: numQubits, gates: gates),
            ),
          ),
        ],
      ),
    );
  }
}

class _CircuitPainter extends CustomPainter {
  final int numQubits;
  final List<Map<String, dynamic>> gates;

  _CircuitPainter({required this.numQubits, required this.gates});

  @override
  void paint(Canvas canvas, Size size) {
    final linePaint = Paint()
      ..color = Colors.white.withOpacity(0.2)
      ..strokeWidth = 1.5;

    // Draw horizontal qubit rails
    for (int q = 0; q < numQubits; q++) {
      final y = 20.0 + q * 40.0;
      canvas.drawLine(Offset(40, y), Offset(size.width, y), linePaint);

      // Label q[i]
      final textSpan = TextSpan(
        text: 'q[$q]',
        style: TextStyle(
          color: Colors.white.withOpacity(0.6),
          fontSize: 11,
          fontFamily: 'monospace',
        ),
      );
      final textPainter = TextPainter(
        text: textSpan,
        textDirection: TextDirection.ltr,
      )..layout();
      textPainter.paint(canvas, Offset(8, y - 6));
    }

    // Draw gates along columns
    double x = 70.0;
    for (final gate in gates) {
      final name = (gate['name'] as String? ?? '').toLowerCase();
      final qubits = (gate['qubits'] as List? ?? [0]).cast<int>();

      if (name == 'cx' && qubits.length >= 2) {
        final qControl = qubits[0];
        final qTarget = qubits[1];
        final y1 = 20.0 + qControl * 40.0;
        final y2 = 20.0 + qTarget * 40.0;

        // Entanglement vertical connector
        final cxLine = Paint()
          ..color = ObservatoryTheme.electricViolet
          ..strokeWidth = 2.0;
        canvas.drawLine(Offset(x, y1), Offset(x, y2), cxLine);

        // Control dot
        final dotPaint = Paint()..color = ObservatoryTheme.electricViolet;
        canvas.drawCircle(Offset(x, y1), 4.5, dotPaint);

        // Target target circle with plus
        final targetPaint = Paint()
          ..color = ObservatoryTheme.electricViolet
          ..style = PaintingStyle.stroke
          ..strokeWidth = 2.0;
        canvas.drawCircle(Offset(x, y2), 9.0, targetPaint);
        canvas.drawLine(Offset(x - 9, y2), Offset(x + 9, y2), cxLine);
        canvas.drawLine(Offset(x, y2 - 9), Offset(x, y2 + 9), cxLine);
      } else {
        final q = qubits.isNotEmpty ? qubits[0] : 0;
        final y = 20.0 + q * 40.0;

        // Gate box
        final boxRect = Rect.fromCenter(center: Offset(x, y), width: 34, height: 26);
        final boxPaint = Paint()
          ..color = ObservatoryTheme.surface3
          ..style = PaintingStyle.fill;
        final borderPaint = Paint()
          ..color = name == 'h'
              ? ObservatoryTheme.cyanCoherence
              : (name.startsWith('r') ? ObservatoryTheme.electricViolet : Colors.white38)
          ..style = PaintingStyle.stroke
          ..strokeWidth = 1.5;

        canvas.drawRRect(RRect.fromRectAndRadius(boxRect, const Radius.circular(4)), boxPaint);
        canvas.drawRRect(RRect.fromRectAndRadius(boxRect, const Radius.circular(4)), borderPaint);

        final label = name.toUpperCase();
        final textSpan = TextSpan(
          text: label,
          style: TextStyle(
            color: Colors.white,
            fontSize: label.length > 2 ? 8 : 10,
            fontWeight: FontWeight.bold,
            fontFamily: 'monospace',
          ),
        );
        final tp = TextPainter(text: textSpan, textDirection: TextDirection.ltr)..layout();
        tp.paint(canvas, Offset(x - tp.width / 2, y - tp.height / 2));
      }
      x += 50.0;
    }
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => true;
}
