// Ref: sections/77-flutter-mobile-app.md — compose drafts persist in
// flutter_secure_storage (small JSON blob) and are restored on reopen.
import 'dart:convert';

import 'package:flutter_secure_storage/flutter_secure_storage.dart';

/// A local compose draft — the offline-safe half of the local+sync
/// strategy. `contentId` links the draft to its server-side contents row
/// once the first save succeeds (later saves PATCH instead of re-creating).
class LocalDraft {
  final String key;
  final String contentType;
  final String title;
  final String body;
  final String? spaceId;
  final String? guideCategory;
  final String? destinationCountry;
  final String? contentId;
  final DateTime updatedAt;

  const LocalDraft({
    required this.key,
    required this.contentType,
    required this.title,
    required this.body,
    this.spaceId,
    this.guideCategory,
    this.destinationCountry,
    this.contentId,
    required this.updatedAt,
  });

  Map<String, dynamic> toJson() => {
        'key': key,
        'content_type': contentType,
        'title': title,
        'body': body,
        'space_id': spaceId,
        'guide_category': guideCategory,
        'destination_country': destinationCountry,
        'content_id': contentId,
        'updated_at': updatedAt.toIso8601String(),
      };

  factory LocalDraft.fromJson(Map<String, dynamic> j) => LocalDraft(
        key: j['key']?.toString() ?? '',
        contentType: j['content_type']?.toString() ?? 'blog_post',
        title: j['title']?.toString() ?? '',
        body: j['body']?.toString() ?? '',
        spaceId: j['space_id']?.toString(),
        guideCategory: j['guide_category']?.toString(),
        destinationCountry: j['destination_country']?.toString(),
        contentId: j['content_id']?.toString(),
        updatedAt: DateTime.tryParse(j['updated_at']?.toString() ?? '') ??
            DateTime.fromMillisecondsSinceEpoch(0),
      );
}

class LocalDraftStore {
  static const _indexKey = 'jc.compose.index';
  static const _prefix = 'jc.compose.draft.';

  final FlutterSecureStorage _storage;
  const LocalDraftStore(this._storage);

  Future<List<String>> _keys() async {
    final raw = await _storage.read(key: _indexKey);
    if (raw == null || raw.isEmpty) return <String>[];
    try {
      return List<String>.from(jsonDecode(raw) as List);
    } catch (_) {
      return const [];
    }
  }

  Future<void> save(LocalDraft draft) async {
    await _storage.write(
        key: '$_prefix${draft.key}', value: jsonEncode(draft.toJson()));
    final keys = await _keys();
    if (!keys.contains(draft.key)) {
      keys.add(draft.key);
      await _storage.write(key: _indexKey, value: jsonEncode(keys));
    }
  }

  Future<LocalDraft?> get(String key) async {
    final raw = await _storage.read(key: '$_prefix$key');
    if (raw == null) return null;
    try {
      return LocalDraft.fromJson(jsonDecode(raw) as Map<String, dynamic>);
    } catch (_) {
      return null;
    }
  }

  Future<List<LocalDraft>> list() async {
    final keys = await _keys();
    final drafts = <LocalDraft>[];
    for (final k in keys) {
      final d = await get(k);
      if (d != null) drafts.add(d);
    }
    drafts.sort((a, b) => b.updatedAt.compareTo(a.updatedAt));
    return drafts;
  }

  Future<void> delete(String key) async {
    await _storage.delete(key: '$_prefix$key');
    final keys = await _keys();
    keys.remove(key);
    await _storage.write(key: _indexKey, value: jsonEncode(keys));
  }
}
