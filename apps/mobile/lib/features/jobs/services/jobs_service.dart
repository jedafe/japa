import 'package:japaconnect_core/japaconnect_core.dart';

class JobsService {
  final JcApiClient _client;
  JobsService(this._client);

  Future<List<Job>> listJobs() async {
    return _client.get<List<Job>>(
      '/jobs',
      parse: (json) =>
          (json as List).map((e) => Job.fromJson(e as Map<String, dynamic>)).toList(),
    );
  }

  Future<Job> getJob(String slug) async {
    final response = await _client.get<Job>(
      '/jobs/$slug',
      parse: (json) => Job.fromJson(json as Map<String, dynamic>),
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
