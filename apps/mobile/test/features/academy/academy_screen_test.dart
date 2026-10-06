import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/academy/screens/course_list_screen.dart';
import 'package:japaconnect_mobile/features/academy/screens/course_detail_screen.dart';
import 'package:japaconnect_mobile/features/academy/screens/lesson_player_screen.dart';

void main() {
  testWidgets('academy screens show loading shimmer', (tester) async {
    for (final screen in [
      const CourseListScreen(),
      const CourseDetailScreen(slug: 'sample'),
      const LessonPlayerScreen(
        trackSlug: 't',
        moduleSlug: 'm',
        lessonSlug: 'l',
      ),
    ]) {
      await tester.pumpWidget(ProviderScope(child: MaterialApp(home: screen)));
      expect(find.byType(LoadingShimmer), findsOneWidget);
    }
  });
}
