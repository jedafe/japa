import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/gamification/screens/badges_screen.dart';
import 'package:japaconnect_mobile/features/gamification/screens/leaderboard_screen.dart';

void main() {
  testWidgets('gamification screens show loading shimmer', (tester) async {
    for (final screen in [
      const BadgesScreen(),
      const LeaderboardScreen(),
    ]) {
      await tester.pumpWidget(
          ProviderScope(child: MaterialApp(home: screen)));
      expect(find.byType(LoadingShimmer), findsOneWidget);
    }
  });
}
