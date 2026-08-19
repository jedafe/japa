import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/forum/screens/forum_list_screen.dart';
import 'package:japaconnect_mobile/features/forum/screens/forum_detail_screen.dart';
import 'package:japaconnect_mobile/features/forum/screens/forum_reply_screen.dart';

void main() {
  testWidgets('forum list screen shows loading shimmer', (tester) async {
    await tester.pumpWidget(
        const ProviderScope(child: MaterialApp(home: ForumListScreen())));
    expect(find.byType(LoadingShimmer), findsOneWidget);
  });

  testWidgets('forum detail screen shows loading shimmer', (tester) async {
    await tester.pumpWidget(const ProviderScope(
        child: MaterialApp(home: ForumDetailScreen(slug: 'sample'))));
    expect(find.byType(LoadingShimmer), findsOneWidget);
  });

  testWidgets('forum reply screen shows reply form', (tester) async {
    await tester.pumpWidget(const ProviderScope(
        child: MaterialApp(home: ForumReplyScreen(slug: 'sample'))));
    expect(find.byType(JcTextField), findsOneWidget);
    expect(find.text('Post'), findsOneWidget);
  });
}
