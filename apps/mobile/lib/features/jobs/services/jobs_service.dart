import 'package:japaconnect_core/japaconnect_core.dart';

class JobsService {
  final JcApiClient _client;
  JobsService(this._client);

  Future<List<Map<String, dynamic>>> listJobs() async {
    return _client.get<List<Map<String, dynamic>>>(
      '/jobs',
      parse: (json) => (json as List).cast<Map<String, dynamic>>(),
    );
  }

  Future<Map<String, dynamic>> getJob(String slug) async {
    final response = await _client.get<Map<String, dynamic>>(
      '/jobs/$slug',
      parse: (json) => json as Map<String, dynamic>,
    );
    return response;
  }

  Future<void> apply(String id) async {
    await _client.post<void>('/jobs/$id/apply', parse: (_) {});
  }

  Future<List<Map<String, dynamic>>> myApplications() async {
    return _client.get<List<Map<String, dynamic>>>(
      '/me/jobs/applications',
      parse: (json) => (json as List).cast<Map<String, dynamic>>(),
    );
  }
}
