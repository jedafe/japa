import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/learning/services/learning_service.dart';

final learningServiceProvider = Provider<LearningService>((ref) {
  final client = ref.watch(jcApiClientProvider);
  return LearningService(client);
});

final courseListProvider =
    FutureProvider.autoDispose<List<Map<String, dynamic>>>((ref) async {
  final service = ref.watch(learningServiceProvider);
  return service.listCourses();
});

final courseDetailProvider = FutureProvider.autoDispose
    .family<Map<String, dynamic>, String>((ref, slug) async {
  final service = ref.watch(learningServiceProvider);
  return service.getCourse(slug);
});

final courseLessonProvider = FutureProvider.autoDispose
    .family<Map<String, dynamic>, (String, String, String)>(
        (ref, args) async {
  final service = ref.watch(learningServiceProvider);
  final (trackSlug, moduleSlug, lessonSlug) = args;
  return service.getLesson(trackSlug, moduleSlug, lessonSlug);
});
