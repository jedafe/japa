import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/profile/screens/profile_screen.dart';
import 'package:japaconnect_mobile/features/profile/screens/settings_screen.dart';
import 'package:japaconnect_mobile/features/profile/screens/wallet_screen.dart';
import 'package:japaconnect_mobile/features/profile/screens/bookmarks_screen.dart';

void main() {
  testWidgets('profile screens show loading shimmer', (tester) async {
    for (final screen in [
      const ProfileScreen(),
      const SettingsScreen(),
      const WalletScreen(),
      const BookmarksScreen(),
    ]) {
      await tester
          .pumpWidget(ProviderScope(child: MaterialApp(home: screen)));
      expect(find.byType(LoadingShimmer), findsOneWidget);
    }
  });
}
