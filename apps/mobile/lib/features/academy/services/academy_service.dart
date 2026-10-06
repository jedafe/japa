import 'package:japaconnect_core/japaconnect_core.dart';

class AcademyService {
  final JcApiClient _client;
  AcademyService(this._client);

  Future<List<Course>> listCourses() async {
    return _client.get<List<Course>>(
      '/academy/catalog',
      parse: (json) => (json as List)
          .map((e) => Course.fromJson(e as Map<String, dynamic>))
          .toList(),
    );
  }

  Future<Course> getCourse(String slug) async {
    return _client.get<Course>(
      '/academy/catalog/$slug',
      parse: (json) => Course.fromJson(json as Map<String, dynamic>),
    );
  }

  Future<void> enroll(String id) async {
    await _client.post<void>('/academy/tracks/$id/enroll', parse: (_) {});
  }

  Future<Lesson> getLesson(
    String trackSlug,
    String moduleSlug,
    String lessonSlug,
  ) async {
    return _client.get<Lesson>(
      '/academy/tracks/$trackSlug/modules/$moduleSlug/lessons/$lessonSlug',
      parse: (json) => Lesson.fromJson(json as Map<String, dynamic>),
    );
  }

  Future<void> updateProgress(String lessonId, double percent) async {
    await _client.post<void>(
      '/academy/lessons/$lessonId/progress',
      body: {'percent': percent},
      parse: (_) {},
    );
  }
}
