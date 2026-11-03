import 'package:flutter_test/flutter_test.dart';
import 'package:japaconnect_mobile/features/compose/drafts/local_draft_store.dart';

void main() {
  group('LocalDraft', () {
    test('toJson/fromJson round-trips all fields', () {
      final draft = LocalDraft(
        key: '1718000000000',
        contentType: 'guide',
        title: 'Moving to Canada',
        body: 'Some **markdown** body',
        spaceId: 'sp-1',
        guideCategory: 'visa',
        destinationCountry: 'CA',
        contentId: 'c-42',
        updatedAt: DateTime.utc(2026, 11, 2, 12),
      );

      final restored = LocalDraft.fromJson(draft.toJson());

      expect(restored.key, draft.key);
      expect(restored.contentType, 'guide');
      expect(restored.title, draft.title);
      expect(restored.body, draft.body);
      expect(restored.spaceId, 'sp-1');
      expect(restored.guideCategory, 'visa');
      expect(restored.destinationCountry, 'CA');
      expect(restored.contentId, 'c-42');
      expect(restored.updatedAt, draft.updatedAt);
    });

    test('fromJson tolerates missing/empty fields', () {
      final d = LocalDraft.fromJson(const {'key': 'k1'});
      expect(d.key, 'k1');
      expect(d.contentType, 'blog_post');
      expect(d.title, '');
      expect(d.spaceId, isNull);
      expect(d.contentId, isNull);
    });

    test('fromJson survives a server form-draft blob shape', () {
      // The /me/form-drafts data payload shares LocalDraft.toJson's shape —
      // a synced draft must restore identically.
      final blob = {
        'key': 'srv',
        'content_type': 'story',
        'title': 'Lagos to Lisbon',
        'body': 'x',
        'updated_at': '2026-11-02T12:00:00.000Z',
      };
      final d = LocalDraft.fromJson(blob);
      expect(d.contentType, 'story');
      expect(d.destinationCountry, isNull);
    });
  });
}
