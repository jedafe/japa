import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';

final jcThemeProvider = Provider<ThemeData>((ref) {
  return ThemeData(
    colorScheme: const ColorScheme.light(primary: JcColors.primary),
    useMaterial3: true,
  );
});
