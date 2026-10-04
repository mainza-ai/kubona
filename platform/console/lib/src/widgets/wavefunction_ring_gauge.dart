import 'dart:math' as math;
import 'package:flutter/material.dart';
import '../theme/observatory_theme.dart';

/// Interactive Tripartite Credibility Wavefunction Ring Gauge.
/// Visualizes basis state probabilities over {|corroborated⟩, |contested⟩, |unverified⟩}.
class WavefunctionRingGauge extends StatelessWidget {
  final double pCorroborated;
  final double pContested;
  final double pUnverified;
  final bool collapsed;
  final String verdict;
  final double size;

  const WavefunctionRingGauge({
    super.key,
    required this.pCorroborated,
    required this.pContested,
    required this.pUnverified,
    required this.collapsed,
    required this.verdict,
    this.size = 140,
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      width: size,
      height: size,
      padding: const EdgeInsets.all(8),
      child: Stack(
        alignment: Alignment.center,
        children: [
          CustomPaint(
            size: Size(size, size),
            painter: _WavefunctionPainter(
              pCorr: pCorroborated,
              pCont: pContested,
              pUnver: pUnverified,
              collapsed: collapsed,
            ),
          ),
          Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(
                verdict.toUpperCase(),
                style: TextStyle(
                  fontSize: 11,
                  fontWeight: FontWeight.bold,
                  letterSpacing: 0.8,
                  color: collapsed
                      ? (verdict == 'corroborated'
                          ? ObservatoryTheme.corroboratedGreen
                          : (verdict == 'contested'
                              ? ObservatoryTheme.contestedCrimson
                              : ObservatoryTheme.unverifiedAmber))
                      : ObservatoryTheme.unverifiedAmber,
                ),
              ),
              const SizedBox(height: 2),
              Text(
                collapsed ? 'COLLAPSED' : 'SUPERPOSED',
                style: TextStyle(
                  fontSize: 9,
                  color: Colors.white.withOpacity(0.5),
                  letterSpacing: 0.5,
                ),
              ),
            ],
          ),
        ],
      ),
    );
  }
}

class _WavefunctionPainter extends CustomPainter {
  final double pCorr;
  final double pCont;
  final double pUnver;
  final bool collapsed;

  _WavefunctionPainter({
    required this.pCorr,
    required this.pCont,
    required this.pUnver,
    required this.collapsed,
  });

  @override
  void paint(Canvas canvas, Size size) {
    final center = Offset(size.width / 2, size.height / 2);
    final radius = size.width / 2 - 10;
    const strokeWidth = 8.0;

    final bgPaint = Paint()
      ..color = Colors.white.withOpacity(0.06)
      ..style = PaintingStyle.stroke
      ..strokeWidth = strokeWidth;

    canvas.drawCircle(center, radius, bgPaint);

    final total = pCorr + pCont + pUnver;
    if (total <= 0) return;

    double startAngle = -math.pi / 2;

    void drawSegment(double p, Color color) {
      if (p <= 0) return;
      final sweepAngle = (p / total) * 2 * math.pi;
      final paint = Paint()
        ..color = color
        ..style = PaintingStyle.stroke
        ..strokeWidth = strokeWidth
        ..strokeCap = StrokeCap.round;

      canvas.drawArc(
        Rect.fromCircle(center: center, radius: radius),
        startAngle,
        sweepAngle,
        false,
        paint,
      );
      startAngle += sweepAngle;
    }

    drawSegment(pCorr, ObservatoryTheme.corroboratedGreen);
    drawSegment(pCont, ObservatoryTheme.contestedCrimson);
    drawSegment(pUnver, ObservatoryTheme.unverifiedAmber);

    // Amber halo if in uncollapsed superposed probe state
    if (!collapsed) {
      final haloPaint = Paint()
        ..color = ObservatoryTheme.unverifiedAmber.withOpacity(0.15)
        ..style = PaintingStyle.stroke
        ..strokeWidth = strokeWidth + 6;
      canvas.drawCircle(center, radius, haloPaint);
    }
  }

  @override
  bool shouldRepaint(covariant _WavefunctionPainter oldDelegate) {
    return oldDelegate.pCorr != pCorr ||
        oldDelegate.pCont != pCont ||
        oldDelegate.pUnver != pUnver ||
        oldDelegate.collapsed != collapsed;
  }
}
