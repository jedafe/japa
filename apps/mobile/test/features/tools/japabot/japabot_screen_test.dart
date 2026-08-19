import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/tools/japabot/screens/japabot_screen.dart';

void main() {
  testWidgets('japabot screen shows loading shimmer', (tester) async {
    await tester.pumpWidget(
        const ProviderScope(child: MaterialApp(home: JapaBotScreen())));
    expect(find.byType(LoadingShimmer), findsOneWidget);
  });
}
