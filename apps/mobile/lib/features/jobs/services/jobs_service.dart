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
    return _client.get<Job>(
      '/jobs/$slug',
      parse: (json) => Job.fromJson(json as Map<String, dynamic>),
    );
  }

  Future<List<JobApplication>> myApplications() async {
    return _client.get<List<JobApplication>>(
      '/me/jobs/applications',
      parse: (json) => (json as List)
          .map((e) => JobApplication.fromJson(e as Map<String, dynamic>))
          .toList(),
    );
  }

  Future<JobApplication> apply(String jobId) async {
    final response = await _client.post<JobApplication>(
      '/jobs/$jobId/apply',
      parse: (json) => JobApplication.fromJson(json as Map<String, dynamic>),
    );
    return response;
  }
}
