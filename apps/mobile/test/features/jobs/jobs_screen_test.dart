import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/jobs/screens/jobs_list_screen.dart';
import 'package:japaconnect_mobile/features/jobs/screens/job_detail_screen.dart';

void main() {
  testWidgets('jobs screens show loading shimmer', (tester) async {
    for (final screen in [
      const JobsListScreen(),
      const JobDetailScreen(slug: 'sample'),
    ]) {
      await tester.pumpWidget(
          ProviderScope(child: MaterialApp(home: screen)));
      expect(find.byType(LoadingShimmer), findsOneWidget);
    }
  });
}
