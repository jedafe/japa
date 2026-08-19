import 'package:japaconnect_core/japaconnect_core.dart';

class VisaChecklistService {
  final JcApiClient _client;
  VisaChecklistService(this._client);

  Future<List<Map<String, dynamic>>> listChecklists() async {
    final response = await _client.get<List<Map<String, dynamic>>>(
      '/checklist/selector',
      parse: (json) => (json as List).cast<Map<String, dynamic>>(),
    );
    return response;
  }

  Future<void> updateProgress(
      String checklistId, String itemId, bool completed) async {
    await _client.patch<void>(
      '/checklist/$checklistId/progress/$itemId',
      body: {'completed': completed},
      parse: (_) {},
    );
  }
}
