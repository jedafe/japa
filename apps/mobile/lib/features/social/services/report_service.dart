import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';

/// Content Reports (§51) — submits user reports against reportable entities.
/// Mirrors the web ReportDialog: entity-specific POST routes into
/// content_reports, reason codes read from the public
/// moderation.report_reasons system setting (Rule 16).
class ReportService {
  final JcApiClient _client;
  ReportService(this._client);

  /// Admin-configured reason codes, falling back to the platform defaults
  /// when the setting is missing or unparseable.
  Future<List<String>> reportReasons() async {
    const fallback = [
      'spam',
      'misinformation',
      'harassment',
      'hate_speech',
      'illegal_content',
      'self_harm',
      'copyright',
      'other',
    ];
    try {
      final res = await _client.get<Map<String, dynamic>>(
        '/site/settings/public',
        parse: (json) => json as Map<String, dynamic>,
      );
      final settings = (res['data'] ?? res) as Map<String, dynamic>;
      final raw = settings['moderation.report_reasons'];
      if (raw is List) {
        final codes = raw.whereType<String>().toList();
        if (codes.isNotEmpty) return codes;
      }
      return fallback;
    } catch (_) {
      return fallback;
    }
  }

  Future<void> _report(
    String path, {
    required String reason,
    String? notes,
    List<String> evidenceUrls = const [],
  }) async {
    await _client.post<Map<String, dynamic>>(
      path,
      body: {
        'reason': reason,
        if (notes != null && notes.trim().isNotEmpty) 'notes': notes.trim(),
        'evidence_urls': evidenceUrls,
      },
      parse: (json) => json as Map<String, dynamic>,
    );
  }

  Future<void> reportContent(String contentId,
          {required String reason, String? notes, List<String> evidenceUrls = const []}) =>
      _report('/content/$contentId/report', reason: reason, notes: notes, evidenceUrls: evidenceUrls);

  Future<void> reportComment(String commentId,
          {required String reason, String? notes, List<String> evidenceUrls = const []}) =>
      _report('/comments/$commentId/report', reason: reason, notes: notes, evidenceUrls: evidenceUrls);

  Future<void> reportUser(String userId,
          {required String reason, String? notes, List<String> evidenceUrls = const []}) =>
      _report('/users/$userId/report', reason: reason, notes: notes, evidenceUrls: evidenceUrls);

  Future<void> reportSpace(String spaceId,
          {required String reason, String? notes, List<String> evidenceUrls = const []}) =>
      _report('/spaces/$spaceId/report', reason: reason, notes: notes, evidenceUrls: evidenceUrls);

  Future<void> reportForumReply(String replyId,
          {required String reason, String? notes, List<String> evidenceUrls = const []}) =>
      _report('/forum/replies/$replyId/report', reason: reason, notes: notes, evidenceUrls: evidenceUrls);

  /// DM message reports — the backend verifies the reporter participates in
  /// the conversation before accepting.
  Future<void> reportDMMessage(String conversationId, String messageId,
          {required String reason, String? notes, List<String> evidenceUrls = const []}) =>
      _report('/dm/conversations/$conversationId/messages/$messageId/report',
          reason: reason, notes: notes, evidenceUrls: evidenceUrls);
}

final reportServiceProvider = Provider<ReportService>((ref) {
  return ReportService(ref.watch(jcApiClientProvider));
});
