import 'package:japaconnect_core/japaconnect_core.dart';

class ScholarshipsService {
  final JcApiClient _client;
  ScholarshipsService(this._client);

  Future<List<Map<String, dynamic>>> listScholarships() async {
    final response = await _client.get<List<Map<String, dynamic>>>(
      '/scholarships',
      parse: (json) => (json as List).cast<Map<String, dynamic>>(),
    );
    return response;
  }

  Future<void> save(String id) async {
    await _client.post<void>('/scholarships/$id/save', parse: (_) {});
  }

  Future<void> unsave(String id) async {
    await _client.delete('/scholarships/$id/save');
  }
}
