// Ref: japaconnect-engineering/sections/77-flutter-mobile-app.md (compose
// drafts), sections/14-content-type-system.md (CRUD + status lifecycle).
import 'package:japaconnect_core/japaconnect_core.dart';

/// A postable space as returned by GET /me/spaces.
class PostableSpace {
  final String id;
  final String name;
  PostableSpace({required this.id, required this.name});
}

/// A guide category option from GET /content-types (the `guide` entry's
/// `categories` array — server-side entity_class = "content_type:guide:{cat}").
class GuideCategory {
  final String slug;
  final String label;
  GuideCategory({required this.slug, required this.label});
}

/// A generic compose draft as stored server-side via /me/form-drafts.
class ServerDraft {
  final String key;
  final Map<String, dynamic> data;
  final String? updatedAt;
  ServerDraft({required this.key, required this.data, this.updatedAt});
}

class ComposeService {
  final JcApiClient _client;
  ComposeService(this._client);

  /// Spaces the caller can post into — drives the primary-space picker
  /// (publish requires a primary space, §14/BUG-007).
  Future<List<PostableSpace>> mySpaces() async {
    return _client.get<List<PostableSpace>>(
      '/me/spaces',
      parse: (json) => (json as List)
          .map((e) => PostableSpace(
              id: (e['id'] ?? e['space_id'] ?? '').toString(),
              name: (e['name'] ?? e['title'] ?? '').toString()))
          .where((s) => s.id.isNotEmpty)
          .toList(),
    );
  }

  /// Guide categories for the category picker, read from the guide entry in
  /// GET /content-types (Rule 16 — never a hardcoded list).
  Future<List<GuideCategory>> guideCategories() async {
    final res = await _client.get<dynamic>('/content-types', parse: (json) => json);
    final list = res is List ? res : (res['data'] as List? ?? const []);
    for (final entry in list) {
      final map = entry as Map<String, dynamic>;
      final slug = (map['value'] ?? map['slug'] ?? map['name'] ?? '').toString();
      if (slug != 'guide') continue;
      final cats = (map['categories'] as List? ?? const []);
      return cats
          .map((c) => GuideCategory(
              slug: (c['Slug'] ?? c['slug'] ?? c['value'] ?? '').toString(),
              label: (c['Label'] ?? c['label'] ?? c['name'] ?? c['slug'] ?? '').toString()))
          .where((c) => c.slug.isNotEmpty)
          .toList();
    }
    return const [];
  }

  /// Create a server-side draft row (POST /content with status=draft).
  /// Returns {id, slug, updated_at} so later saves PATCH the same row.
  Future<Map<String, dynamic>> createDraft(Map<String, dynamic> body) async {
    return _client.post<Map<String, dynamic>>(
      '/content',
      body: {...body, 'status': 'draft'},
      parse: (json) => Map<String, dynamic>.from(json as Map),
    );
  }

  Future<void> updateDraft(String id, Map<String, dynamic> body) async {
    await _client.patch<dynamic>('/content/$id',
        body: body, parse: (json) => json);
  }

  /// POST /content/{id}/publish — the backend applies the story review
  /// clamp (§79) and the require_content_review setting; the response
  /// status tells the UI whether it went live or to the queue.
  Future<String> publish(String id) async {
    final res = await _client.post<Map<String, dynamic>>(
      '/content/$id/publish',
      parse: (json) => Map<String, dynamic>.from(json as Map),
    );
    return (res['status'] ?? 'published').toString();
  }

  /// Forum threads have their own endpoint (POST /forum — forum.thread.create
  /// node), not the generic /content route.
  Future<Map<String, dynamic>> createThread(
      {required String title, required String body, String? spaceId}) async {
    return _client.post<Map<String, dynamic>>(
      '/forum',
      body: {
        'title': title,
        'body': body,
        if (spaceId != null && spaceId.isNotEmpty) 'space_id': spaceId,
      },
      parse: (json) => Map<String, dynamic>.from(json as Map),
    );
  }

  /// Server-side form drafts (kind=compose) — the cross-device half of the
  /// local+sync draft strategy (§77 local blob + /me/form-drafts sync).
  Future<List<ServerDraft>> serverDrafts() async {
    return _client.get<List<ServerDraft>>(
      '/me/form-drafts',
      parse: (json) => (json as List)
          .where((e) => (e['kind'] ?? '') == 'compose')
          .map((e) => ServerDraft(
              key: (e['key'] ?? '').toString(),
              data: Map<String, dynamic>.from((e['data'] as Map?) ?? const {}),
              updatedAt: e['updated_at']?.toString()))
          .toList(),
    );
  }

  Future<void> putServerDraft(String key, Map<String, dynamic> data) async {
    await _client.put<dynamic>('/me/form-drafts/compose/$key',
        body: {'data': data}, parse: (json) => json);
  }

  Future<void> deleteServerDraft(String key) async {
    await _client.delete('/me/form-drafts/compose/$key');
  }
}
