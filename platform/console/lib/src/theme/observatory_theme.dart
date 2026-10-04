import 'package:flutter/material.dart';

/// Quantum Epistemic Observatory visual styling tokens.
class ObservatoryTheme {
  // Void canvas background & surface elevations
  static const Color voidBg = Color(0xFF090C10);
  static const Color surface1 = Color(0xFF0E131F);
  static const Color surface2 = Color(0xFF161D2F);
  static const Color surface3 = Color(0xFF1E2842);

  // Quantum coherence accents
  static const Color cyanCoherence = Color(0xFF00F2FE);
  static const Color cyanDim = Color(0xFF0398A8);
  static const Color electricViolet = Color(0xFF8B5CF6);
  static const Color electricVioletDim = Color(0xFF6D28D9);

  // Epistemic states
  static const Color corroboratedGreen = Color(0xFF10B981);
  static const Color contestedCrimson = Color(0xFFEF4444);
  static const Color unverifiedAmber = Color(0xFFF59E0B);

  // Borders & Dividers
  static const Color borderSubtle = Color(0x1FFFFFFF);
  static const Color borderCyan = Color(0x3300F2FE);
  static const Color borderViolet = Color(0x338B5CF6);

  static ThemeData get darkTheme {
    return ThemeData(
      brightness: Brightness.dark,
      scaffoldBackgroundColor: voidBg,
      primaryColor: cyanCoherence,
      colorScheme: const ColorScheme.dark(
        primary: cyanCoherence,
        secondary: electricViolet,
        surface: surface1,
        error: contestedCrimson,
      ),
      cardTheme: CardThemeData(
        color: surface1,
        elevation: 0,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(10),
          side: const BorderSide(color: borderSubtle, width: 1),
        ),
      ),
      appBarTheme: const AppBarTheme(
        backgroundColor: surface1,
        elevation: 0,
        centerTitle: false,
        titleTextStyle: TextStyle(
          color: Colors.white,
          fontSize: 18,
          fontWeight: FontWeight.w600,
          letterSpacing: 0.5,
        ),
      ),
      tabBarTheme: const TabBarThemeData(
        indicatorColor: cyanCoherence,
        indicatorSize: TabBarIndicatorSize.tab,
        labelColor: cyanCoherence,
        unselectedLabelColor: Colors.white54,
        labelStyle: TextStyle(fontWeight: FontWeight.w600, fontSize: 13),
      ),
      dividerColor: borderSubtle,
    );
  }
}
