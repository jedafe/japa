import 'package:japaconnect_core/japaconnect_core.dart';

/// Service for the Japa Readiness Assessment (§71, ADR-071).
///
/// The backend stores and scores submissions asynchronously; the UI polls
/// the result-by-submission endpoint until scoring completes.
class ReadinessService {
  final JcApiClient _client;
  ReadinessService(this._client);

  Future<Map<String, dynamic>> getAssessment() async {
    final response = await _client.get<Map<String, dynamic>>(
      '/assessment',
      parse: (json) => json as Map<String, dynamic>,
    );
    return response;
  }

  Future<Map<String, dynamic>> submit(Map<String, dynamic> fieldValues) async {
    final response = await _client.post<Map<String, dynamic>>(
      '/assessment/submit',
      body: {'field_values': fieldValues},
      parse: (json) => json as Map<String, dynamic>,
    );
    return response;
  }

  Future<Map<String, dynamic>> getResultBySubmission(String submissionId) async {
    final response = await _client.get<Map<String, dynamic>>(
      '/assessment/result/by-submission/$submissionId',
      parse: (json) => json as Map<String, dynamic>,
    );
    return response;
  }

  Future<Map<String, dynamic>> getResult(String resultId) async {
    final response = await _client.get<Map<String, dynamic>>(
      '/assessment/result/$resultId',
      parse: (json) => json as Map<String, dynamic>,
    );
    return response;
  }

  Future<List<dynamic>> getHistory() async {
    final response = await _client.get<List<dynamic>>(
      '/assessment/history?limit=50',
      parse: (json) => json as List<dynamic>,
    );
    return response;
  }
}
