import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/social/screens/notifications_screen.dart';
import 'package:japaconnect_mobile/features/social/screens/messages_list_screen.dart';
import 'package:japaconnect_mobile/features/social/screens/message_thread_screen.dart';

void main() {
  testWidgets('social screens show loading shimmer', (tester) async {
    for (final screen in [
      const NotificationsScreen(),
      const MessagesListScreen(),
      const MessageThreadScreen(id: '1'),
    ]) {
      await tester
          .pumpWidget(ProviderScope(child: MaterialApp(home: screen)));
      expect(find.byType(LoadingShimmer), findsOneWidget);
    }
  });
}
