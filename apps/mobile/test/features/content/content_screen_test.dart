import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/content/screens/blog_list_screen.dart';
import 'package:japaconnect_mobile/features/content/screens/blog_detail_screen.dart';
import 'package:japaconnect_mobile/features/content/screens/guide_list_screen.dart';
import 'package:japaconnect_mobile/features/content/screens/comparison_list_screen.dart';

void main() {
  testWidgets('content list screens show loading shimmer', (tester) async {
    for (final screen in [
      const BlogListScreen(),
      const GuideListScreen(),
      const ComparisonListScreen(),
      const BlogDetailScreen(slug: 'sample'),
    ]) {
      await tester.pumpWidget(
          ProviderScope(child: MaterialApp(home: screen)));
      expect(find.byType(LoadingShimmer), findsOneWidget);
    }
  });
}
