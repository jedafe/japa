import 'package:japaconnect_core/japaconnect_core.dart';

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

  Future<Map<String, dynamic>> submit(Map<String, dynamic> answers) async {
    final response = await _client.post<Map<String, dynamic>>(
      '/assessment/submit',
      body: answers,
      parse: (json) => json as Map<String, dynamic>,
    );
    return response;
  }
}
