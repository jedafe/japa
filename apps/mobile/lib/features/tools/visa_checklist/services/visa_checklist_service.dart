import 'package:japaconnect_core/japaconnect_core.dart';

/// §70 visa checklist contract — selector → guide → progress.
/// GET /checklist/selector returns an OBJECT ({origins, destinations}),
/// each destination carrying visa_types[{value,label,guide_id}]. The old
/// service parsed that object as a List and the screen consumed fields
/// that never existed — both rewritten against the real shape.

class ChecklistVisaTypeOption {
  final String value;
  final String label;
  final String guideId;

  const ChecklistVisaTypeOption({
    required this.value,
    required this.label,
    required this.guideId,
  });

  factory ChecklistVisaTypeOption.fromJson(Map<String, dynamic> json) =>
      ChecklistVisaTypeOption(
        value: (json['value'] ?? '').toString(),
        label: (json['label'] ?? '').toString(),
        guideId: (json['guide_id'] ?? '').toString(),
      );
}

class ChecklistCountryOption {
  final String id;
  final String name;
  final String slug;
  final String code;
  final List<ChecklistVisaTypeOption> visaTypes;

  const ChecklistCountryOption({
    required this.id,
    required this.name,
    required this.slug,
    this.code = '',
    this.visaTypes = const [],
  });

  factory ChecklistCountryOption.fromJson(Map<String, dynamic> json) =>
      ChecklistCountryOption(
        id: (json['id'] ?? '').toString(),
        name: (json['name'] ?? '').toString(),
        slug: (json['slug'] ?? '').toString(),
        code: (json['code'] ?? '').toString(),
        visaTypes: ((json['visa_types'] as List?) ?? const [])
            .whereType<Map<String, dynamic>>()
            .map(ChecklistVisaTypeOption.fromJson)
            .toList(),
      );
}

class ChecklistSelector {
  final List<ChecklistCountryOption> origins;
  final List<ChecklistCountryOption> destinations;

  const ChecklistSelector({
    this.origins = const [],
    this.destinations = const [],
  });

  factory ChecklistSelector.fromJson(Map<String, dynamic> json) =>
      ChecklistSelector(
        origins: ((json['origins'] as List?) ?? const [])
            .whereType<Map<String, dynamic>>()
            .map(ChecklistCountryOption.fromJson)
            .toList(),
        destinations: ((json['destinations'] as List?) ?? const [])
            .whereType<Map<String, dynamic>>()
            .map(ChecklistCountryOption.fromJson)
            .toList(),
      );
}

class ChecklistItem {
  final String itemKey;
  final String label;
  final String description;
  final bool required;
  final String category;
  final bool completed;
  final String notes;

  const ChecklistItem({
    required this.itemKey,
    required this.label,
    this.description = '',
    this.required = false,
    this.category = '',
    this.completed = false,
    this.notes = '',
  });

  factory ChecklistItem.fromJson(Map<String, dynamic> json) => ChecklistItem(
    itemKey: (json['item_key'] ?? '').toString(),
    label: (json['label'] ?? '').toString(),
    description: (json['description'] ?? '').toString(),
    required: json['required'] == true,
    category: (json['category'] ?? '').toString(),
    completed: json['completed'] == true,
    notes: (json['notes'] ?? '').toString(),
  );

  ChecklistItem copyWith({bool? completed, String? notes}) => ChecklistItem(
    itemKey: itemKey,
    label: label,
    description: description,
    required: required,
    category: category,
    completed: completed ?? this.completed,
    notes: notes ?? this.notes,
  );
}

class ChecklistGuide {
  final String guideId;
  final String title;
  final List<ChecklistItem> items;

  const ChecklistGuide({
    required this.guideId,
    required this.title,
    this.items = const [],
  });

  factory ChecklistGuide.fromJson(Map<String, dynamic> json) => ChecklistGuide(
    guideId: (json['guide_id'] ?? '').toString(),
    title: (json['title'] ?? '').toString(),
    items: ((json['items'] as List?) ?? const [])
        .whereType<Map<String, dynamic>>()
        .map(ChecklistItem.fromJson)
        .toList(),
  );
}

class ChecklistProgressItem {
  final String itemKey;
  final bool completed;
  final String notes;

  const ChecklistProgressItem({
    required this.itemKey,
    this.completed = false,
    this.notes = '',
  });

  factory ChecklistProgressItem.fromJson(Map<String, dynamic> json) =>
      ChecklistProgressItem(
        itemKey: (json['item_key'] ?? '').toString(),
        completed: json['completed'] == true,
        notes: (json['notes'] ?? '').toString(),
      );
}

class VisaChecklistService {
  final JcApiClient _client;
  VisaChecklistService(this._client);

  Map<String, dynamic> _originQuery(String? origin, String? originCode) {
    final q = <String, dynamic>{};
    if (origin != null && origin.isNotEmpty) q['origin'] = origin;
    if (originCode != null && originCode.isNotEmpty) {
      q['origin_code'] = originCode;
    }
    return q;
  }

  /// JcApiClient.patch has no `query` parameter — encode origin params onto
  /// the path so progress writes land in the same origin-scoped view.
  String _withOriginQuery(String path, String? origin, String? originCode) {
    final q = _originQuery(origin, originCode);
    if (q.isEmpty) return path;
    final qs = q.entries
        .map(
          (e) =>
              '${Uri.encodeQueryComponent(e.key)}=${Uri.encodeQueryComponent(e.value.toString())}',
        )
        .join('&');
    return '$path?$qs';
  }

  /// GET /checklist/selector — 3-step selector options (public).
  Future<ChecklistSelector> getSelector() async {
    return _client.get<ChecklistSelector>(
      '/checklist/selector',
      parse: (json) =>
          ChecklistSelector.fromJson(Map<String, dynamic>.from(json as Map)),
    );
  }

  /// GET /checklist/{guide_id} — items for the picked route; `origin`
  /// narrows the set to origin-specific requirements (ADR-070 §2).
  Future<ChecklistGuide> getGuide(
    String guideId, {
    String? origin,
    String? originCode,
  }) async {
    return _client.get<ChecklistGuide>(
      '/checklist/$guideId',
      query: _originQuery(origin, originCode),
      parse: (json) =>
          ChecklistGuide.fromJson(Map<String, dynamic>.from(json as Map)),
    );
  }

  /// GET /checklist/{guide_id}/progress — authenticated user's saved state.
  Future<List<ChecklistProgressItem>> getProgress(
    String guideId, {
    String? origin,
    String? originCode,
  }) async {
    return _client.get<List<ChecklistProgressItem>>(
      '/checklist/$guideId/progress',
      query: _originQuery(origin, originCode),
      parse: (json) =>
          ((json as Map<String, dynamic>?)?['items'] as List? ?? const [])
              .whereType<Map<String, dynamic>>()
              .map(ChecklistProgressItem.fromJson)
              .toList(),
    );
  }

  /// PATCH /checklist/{guide_id}/progress/{item_key} — toggle a document or
  /// update its note (auth required — checklist.use).
  Future<void> updateProgress(
    String guideId,
    String itemKey, {
    bool? completed,
    String? notes,
    String? origin,
    String? originCode,
  }) async {
    await _client.patch<void>(
      _withOriginQuery(
        '/checklist/$guideId/progress/$itemKey',
        origin,
        originCode,
      ),
      body: {
        'completed': ?completed,
        'notes': ?notes,
      },
      parse: (_) {},
    );
  }
}
