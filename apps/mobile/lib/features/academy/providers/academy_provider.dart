import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/academy/services/academy_service.dart';

final academyServiceProvider = Provider<AcademyService>((ref) {
  final client = ref.watch(jcApiClientProvider);
  return AcademyService(client);
});

final courseListProvider = FutureProvider.autoDispose<List<Course>>((
  ref,
) async {
  final service = ref.watch(academyServiceProvider);
  return service.listCourses();
});

final courseDetailProvider = FutureProvider.autoDispose.family<Course, String>((
  ref,
  slug,
) async {
  final service = ref.watch(academyServiceProvider);
  return service.getCourse(slug);
});

final courseLessonProvider = FutureProvider.autoDispose
    .family<Lesson, (String, String, String)>((ref, args) async {
      final service = ref.watch(academyServiceProvider);
      final (trackSlug, moduleSlug, lessonSlug) = args;
      return service.getLesson(trackSlug, moduleSlug, lessonSlug);
    });
