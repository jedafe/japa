import 'package:japaconnect_core/japaconnect_core.dart';

class LearningService {
  final JcApiClient _client;
  LearningService(this._client);

  Future<List<Course>> listCourses() async {
    return _client.get<List<Course>>(
      '/learn/university/catalog',
      parse: (json) =>
          (json as List).map((e) => Course.fromJson(e as Map<String, dynamic>)).toList(),
    );
  }

  Future<Course> getCourse(String slug) async {
    return _client.get<Course>(
      '/learn/university/catalog/$slug',
      parse: (json) => Course.fromJson(json as Map<String, dynamic>),
    );
  }

  Future<void> enroll(String id) async {
    await _client.post<void>(
      '/learn/tracks/$id/enroll',
      parse: (_) {},
    );
  }

  Future<Lesson> getLesson(
    String trackSlug,
    String moduleSlug,
    String lessonSlug,
  ) async {
    return _client.get<Lesson>(
      '/learn/tracks/$trackSlug/modules/$moduleSlug/lessons/$lessonSlug',
      parse: (json) => Lesson.fromJson(json as Map<String, dynamic>),
    );
  }

  Future<void> updateProgress(String lessonId, double percent) async {
    await _client.post<void>(
      '/learn/lessons/$lessonId/progress',
      body: {'percent': percent},
      parse: (_) {},
    );
  }
}
