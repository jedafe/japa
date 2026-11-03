// Ref: sections/77-flutter-mobile-app.md — mobile composer (BUG-CE-023).
// Types: blog_post, story, guide, discussion (forum thread). Discussion
// submits via POST /forum; the rest via /content draft → publish.
import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/compose/drafts/local_draft_store.dart';
import 'package:japaconnect_mobile/features/compose/providers/compose_provider.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';

class ComposeScreen extends ConsumerStatefulWidget {
  /// Local draft key to resume (from /compose?draft={key}).
  final String? draftKey;
  const ComposeScreen({super.key, this.draftKey});

  @override
  ConsumerState<ComposeScreen> createState() => _ComposeScreenState();
}

class _ComposeScreenState extends ConsumerState<ComposeScreen> {
  static const _types = ['blog_post', 'story', 'guide', 'discussion'];

  late final TextEditingController _titleCtrl;
  late final TextEditingController _bodyCtrl;
  late String _draftKey;
  String _type = 'blog_post';
  String? _spaceId;
  String? _guideCategory;
  String? _destCountry;
  String? _contentId;
  bool _sending = false;
  Timer? _autosave;

  @override
  void initState() {
    super.initState();
    _draftKey = widget.draftKey ?? DateTime.now().millisecondsSinceEpoch.toString();
    _titleCtrl = TextEditingController();
    _bodyCtrl = TextEditingController();
    // Listeners keep _canPublish re-evaluated and feed the autosave —
    // JcTextField exposes no onChanged, so the controller drives it.
    _titleCtrl.addListener(_fieldChanged);
    _bodyCtrl.addListener(_fieldChanged);
    if (widget.draftKey != null) _resumeDraft();
  }

  Future<void> _resumeDraft() async {
    var draft = await ref.read(localDraftStoreProvider).get(_draftKey);
    if (draft == null) {
      // Server-only draft (synced from another device) — the form-drafts
      // blob carries the same shape as LocalDraft.toJson.
      try {
        final server = await ref.read(composeServiceProvider).serverDrafts();
        final match = server.where((d) => d.key == _draftKey).firstOrNull;
        if (match != null) draft = LocalDraft.fromJson(match.data);
      } catch (_) {}
    }
    if (draft == null || !mounted) return;
    final d = draft;
    setState(() {
      _type = _types.contains(d.contentType) ? d.contentType : 'blog_post';
      _titleCtrl.text = d.title;
      _bodyCtrl.text = d.body;
      _spaceId = d.spaceId;
      _guideCategory = d.guideCategory;
      _destCountry = d.destinationCountry;
      _contentId = d.contentId;
    });
  }

  @override
  void dispose() {
    _autosave?.cancel();
    _titleCtrl.dispose();
    _bodyCtrl.dispose();
    super.dispose();
  }

  /// §77 offline policy: autosave the draft blob to secure storage on every
  /// field change (debounced). The server sync happens on explicit save.
  void _fieldChanged() {
    setState(() {});
    _onChanged();
  }

  void _onChanged() {
    _autosave?.cancel();
    _autosave = Timer(const Duration(milliseconds: 800), _persistLocal);
  }

  LocalDraft _snapshot() => LocalDraft(
        key: _draftKey,
        contentType: _type,
        title: _titleCtrl.text.trim(),
        body: _bodyCtrl.text.trim(),
        spaceId: _spaceId,
        guideCategory: _guideCategory,
        destinationCountry: _destCountry,
        contentId: _contentId,
        updatedAt: DateTime.now(),
      );

  Future<void> _persistLocal() async {
    if (!mounted) return;
    await ref.read(localDraftStoreProvider).save(_snapshot());
  }

  Map<String, dynamic> _contentBody() => {
        'title': _titleCtrl.text.trim(),
        'body': _bodyCtrl.text.trim(),
        'content_type': _type,
        if (_spaceId != null && _spaceId!.isNotEmpty) 'space_id': _spaceId,
        if (_type == 'guide' && _guideCategory != null)
          'guide_category': _guideCategory,
        // Story destination is a field_values key (store.go GeoFieldValue
        // reads destination_country/target_country/country from EAV, which
        // drives story_meta + §79 destination-space federation) — not a
        // top-level input field.
        if (_type == 'story' && _destCountry != null)
          'field_values': {'destination_country': _destCountry},
      };

  /// Save draft server-side (POST once, PATCH after) + sync the form-draft
  /// blob so the draft survives on other devices. Discussion has no
  /// /content row — forum threads only exist on publish via POST /forum —
  /// so only the local + form-draft blob is saved for that type.
  Future<bool> _saveDraft() async {
    final service = ref.read(composeServiceProvider);
    try {
      if (_type != 'discussion') {
        final body = _contentBody();
        if (_contentId == null) {
          final res = await service.createDraft(body);
          _contentId = res['id']?.toString();
        } else {
          await service.updateDraft(_contentId!, body);
        }
      }
      await service.putServerDraft(_draftKey, _snapshot().toJson());
      return true;
    } catch (_) {
      return false;
    }
  }

  Future<void> _save() async {
    if (_sending) return;
    setState(() => _sending = true);
    final t = ref.read(tProvider);
    final ok = await _saveDraft();
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(
        content: Text(ok ? t('compose.draft_saved') : t('compose.save_failed'))));
    setState(() => _sending = false);
  }

  bool get _canPublish {
    if (_titleCtrl.text.trim().isEmpty || _bodyCtrl.text.trim().isEmpty) {
      return false;
    }
    if (_spaceId == null || _spaceId!.isEmpty) {
      return false; // publish requires a primary space (all types)
    }
    if (_type == 'guide' && _guideCategory == null) return false;
    if (_type == 'story' && _destCountry == null) return false;
    return true;
  }

  Future<void> _publish() async {
    if (_sending || !_canPublish) return;
    setState(() => _sending = true);
    final t = ref.read(tProvider);
    try {
      String status;
      if (_type == 'discussion') {
        await ref.read(composeServiceProvider).createThread(
            title: _titleCtrl.text.trim(),
            body: _bodyCtrl.text.trim(),
            spaceId: _spaceId);
        status = 'published';
      } else {
        if (!await _saveDraft() || _contentId == null) {
          throw Exception('draft save failed');
        }
        status = await ref.read(composeServiceProvider).publish(_contentId!);
      }
      await ref.read(localDraftStoreProvider).delete(_draftKey);
      await ref.read(composeServiceProvider).deleteServerDraft(_draftKey);
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(
          content: Text(status == 'in_review'
              ? t('compose.submitted_review')
              : t('compose.published'))));
      context.pop();
    } catch (_) {
      if (!mounted) return;
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text(t('compose.publish_failed'))));
      setState(() => _sending = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final t = ref.watch(tProvider);
    final spaces = ref.watch(postableSpacesProvider);
    final guideCats = ref.watch(guideCategoriesProvider);
    final countries = ref.watch(countriesProvider);

    return Scaffold(
      appBar: AppBar(title: Text(t('compose.title'))),
      body: SingleChildScrollView(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Wrap(
              spacing: 8,
              children: _types
                  .map((ty) => ChoiceChip(
                      label: Text(t('compose.type.$ty')),
                      selected: _type == ty,
                      onSelected: (_) => setState(() {
                            _type = ty;
                            _onChanged();
                          })))
                  .toList(),
            ),
            const SizedBox(height: 16),
            JcTextField(label: t('compose.field.title'), controller: _titleCtrl),
            const SizedBox(height: 12),
            TextField(
              controller: _bodyCtrl,
              minLines: 8,
              maxLines: null,
              decoration: InputDecoration(
                  labelText: t('compose.field.body'), border: const OutlineInputBorder()),
            ),
            const SizedBox(height: 12),
            spaces.when(
              data: (list) => DropdownButtonFormField<String>(
                initialValue: _spaceId,
                decoration:
                    InputDecoration(labelText: t('compose.field.space')),
                items: list
                    .map((s) => DropdownMenuItem(value: s.id, child: Text(s.name)))
                    .toList(),
                onChanged: (v) => setState(() {
                  _spaceId = v;
                  _onChanged();
                }),
              ),
              loading: () => const LinearProgressIndicator(),
              error: (_, _) => const SizedBox.shrink(),
            ),
            if (_type == 'guide') ...[
              const SizedBox(height: 12),
              guideCats.when(
                data: (list) => DropdownButtonFormField<String>(
                  initialValue: _guideCategory,
                  decoration:
                      InputDecoration(labelText: t('compose.field.category')),
                  items: list
                      .map((c) =>
                          DropdownMenuItem(value: c.slug, child: Text(c.label)))
                      .toList(),
                  onChanged: (v) => setState(() {
                    _guideCategory = v;
                    _onChanged();
                  }),
                ),
                loading: () => const LinearProgressIndicator(),
                error: (_, _) => const SizedBox.shrink(),
              ),
            ],
            if (_type == 'story') ...[
              const SizedBox(height: 12),
              countries.when(
                data: (list) => DropdownButtonFormField<String>(
                  initialValue: _destCountry,
                  decoration:
                      InputDecoration(labelText: t('compose.field.destination')),
                  items: list
                      .map((c) => DropdownMenuItem(
                          value: c.id, child: Text(c.name)))
                      .toList(),
                  onChanged: (v) => setState(() {
                    _destCountry = v;
                    _onChanged();
                  }),
                ),
                loading: () => const LinearProgressIndicator(),
                error: (_, _) => const SizedBox.shrink(),
              ),
            ],
            const SizedBox(height: 24),
            Row(
              children: [
                Expanded(
                  child: JcButton(
                    label: _sending
                        ? t('compose.saving')
                        : t('compose.save_draft'),
                    onPressed: _sending ? null : _save,
                  ),
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: JcButton(
                    label: _type == 'discussion'
                        ? t('compose.post')
                        : t('compose.publish'),
                    onPressed: (_sending || !_canPublish) ? null : _publish,
                  ),
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }
}
