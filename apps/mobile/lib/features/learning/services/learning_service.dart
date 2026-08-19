import 'package:japaconnect_core/japaconnect_core.dart';

class LearningService {
  final JcApiClient _client;
  LearningService(this._client);

  Future<List<Map<String, dynamic>>> listCourses() async {
    return _client.get<List<Map<String, dynamic>>>(
      '/learn/university/catalog',
      parse: (json) => (json as List).cast<Map<String, dynamic>>(),
    );
  }

  Future<Map<String, dynamic>> getCourse(String slug) async {
    return _client.get<Map<String, dynamic>>(
      '/learn/university/catalog/$slug',
      parse: (json) => json as Map<String, dynamic>,
    );
  }

  Future<void> enroll(String id) async {
    await _client.post<void>(
      '/learn/tracks/$id/enroll',
      parse: (_) {},
    );
  }

  Future<Map<String, dynamic>> getLesson(
    String trackSlug,
    String moduleSlug,
    String lessonSlug,
  ) async {
    return _client.get<Map<String, dynamic>>(
      '/learn/tracks/$trackSlug/modules/$moduleSlug/lessons/$lessonSlug',
      parse: (json) => json as Map<String, dynamic>,
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
