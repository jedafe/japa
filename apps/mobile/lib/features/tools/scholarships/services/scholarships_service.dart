import 'package:japaconnect_core/japaconnect_core.dart';

class ScholarshipsService {
  final JcApiClient _client;
  ScholarshipsService(this._client);

  Future<List<Scholarship>> listScholarships() async {
    final response = await _client.get<List<Scholarship>>(
      '/scholarships',
      parse: (json) =>
          (json as List).map((e) => Scholarship.fromJson(e as Map<String, dynamic>)).toList(),
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
