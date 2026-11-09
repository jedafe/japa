/// §06 conditional form rules — per-field visible_if conditions, per-step
/// visible_if conditions, and forward-only jump rules. Semantics are mirrored
/// 1:1 from internal/formlogic (Go) and packages/formlogic (TS) — keep all
/// three in sync.
/// Ref: japaconnect-engineering/sections/06-custom-field-builder-form-builder.md (ADR-137)
/// Ref: docs/superpowers/specs/2026-11-09-conditional-form-logic-design.md
library form_logic;

typedef Answers = Map<String, dynamic>;

bool _isAnswered(dynamic v, bool ok) {
  if (!ok || v == null) return false;
  if (v is String) return v.trim().isNotEmpty;
  if (v is List) return v.isNotEmpty;
  return true;
}

bool? _toBool(dynamic v) {
  if (v is bool) return v;
  if (v is String) {
    switch (v.toLowerCase()) {
      case 'true':
      case '1':
      case 'yes':
      case 'on':
        return true;
      case 'false':
      case '0':
      case 'no':
      case 'off':
        return false;
    }
  }
  return null;
}

double? _toFloat(dynamic v) {
  if (v is num) return v.isFinite ? v.toDouble() : null;
  if (v is String) return double.tryParse(v.trim());
  return null;
}

bool _scalarEqual(dynamic a, dynamic b) {
  final ab = _toBool(a);
  if (ab != null) {
    final bb = _toBool(b);
    if (bb != null) return ab == bb;
  }
  final af = _toFloat(a);
  if (af != null) {
    final bf = _toFloat(b);
    if (bf != null) return af == bf;
  }
  return '$a' == '$b';
}

bool _evalClause(Map<String, dynamic>? cl, Answers answers, String selfKey) {
  // Malformed (non-map) clause → false, matching the Go/TS evaluators'
  // zero-value Clause{} — a broken clause must not silently pass.
  if (cl == null) return false;
  final f = cl['field'];
  final key = f is String && f.isNotEmpty ? f : selfKey;
  final present = answers.containsKey(key);
  final has = _isAnswered(answers[key], present);

  switch (cl['op']) {
    case 'answered':
      return has;
    case 'not_answered':
      return !has;
  }
  if (!has) return false;

  switch (cl['op']) {
    case 'eq':
      return _scalarEqual(answers[key], cl['value']);
    case 'neq':
      return !_scalarEqual(answers[key], cl['value']);
    case 'in':
      final arr = cl['value'];
      if (arr is! List) return false;
      return arr.any((item) => _scalarEqual(answers[key], item));
    case 'contains':
      final arr = answers[key];
      if (arr is! List) return false;
      return arr.any((el) => _scalarEqual(el, cl['value']));
  }
  return false; // unknown op → clause false
}

/// Reports whether the condition holds for [answers]:
/// AND(all) && (no any-group || OR(any)). Absent/malformed → true (fail-open).
bool evalCondition(Map<String, dynamic>? cond, Answers answers, [String selfKey = '']) {
  if (cond == null) return true;
  final all = cond['all'];
  if (all is List) {
    for (final cl in all) {
      if (!_evalClause(cl is Map ? cl.cast<String, dynamic>() : null, answers, selfKey)) {
        return false;
      }
    }
  }
  final any = cond['any'];
  if (any is List && any.isNotEmpty) {
    for (final cl in any) {
      if (_evalClause(cl is Map ? cl.cast<String, dynamic>() : null, answers, selfKey)) {
        return true;
      }
    }
    return false;
  }
  return true;
}

/// Reports whether a field's own visible_if passes.
/// Absent/malformed config → visible (fail-open).
bool fieldVisible(Map<String, dynamic>? config, Answers answers, [String selfKey = '']) {
  if (config == null) return true;
  final vis = config['visible_if'];
  return evalCondition(vis is Map ? vis.cast<String, dynamic>() : null, answers, selfKey);
}

/// Reports whether a step's visible_if passes (null → visible).
bool stepVisible(Map<String, dynamic>? config, Answers answers) {
  if (config == null) return true;
  final vis = config['visible_if'];
  return evalCondition(vis is Map ? vis.cast<String, dynamic>() : null, answers, '');
}

/// Filters a field list by visible_if against current answers. Each entry
/// must expose `key` and may expose `config`.
List<Map<String, dynamic>> visibleFields(List<dynamic> fields, Answers answers) {
  return fields.whereType<Map>().map((f) => f.cast<String, dynamic>()).where((f) {
    final k = f['key'];
    final key = k is String ? k : '';
    final cfg = f['config'];
    return fieldVisible(cfg is Map ? cfg.cast<String, dynamic>() : null, answers, key);
  }).toList();
}

/// Returns the step keys hidden by triggered forward-only jumps.
/// [stepOrder] lists every step identifier in natural order; [stepHidden]
/// seeds the result with steps hidden by other mechanisms (their own
/// visible_if). A jump fires when its field passes fieldVisible, its
/// containing step is not already hidden, and jump.when holds; an absent or
/// empty when fires iff the owning field is answered (a jump is
/// answer-driven — an unconditional interval skip is an authoring error).
/// Sources are processed in natural step order so a field inside a step
/// hidden by an earlier jump can never fire its own; it hides the open
/// interval (source, target). Backward/self/unknown targets are ignored;
/// multiple triggered jumps union their hidden intervals.
///
/// Each field entry is a decoded API map: `key`, `config.dimension` (the
/// containing step's key for assessment forms — pass [stepKeyOf] to override
/// for other shapes), `config.jump`.
Set<String> hiddenStepsByJump(
  List<dynamic> fields,
  List<String> stepOrder,
  Answers answers, {
  String Function(Map<String, dynamic> field)? stepKeyOf,
  Set<String>? stepHidden,
}) {
  String defaultStepKey(Map<String, dynamic> f) {
    final cfg = f['config'];
    if (cfg is Map) {
      final dim = cfg['dimension'];
      if (dim is String && dim.isNotEmpty) return dim;
    }
    return '';
  }

  final keyOf = stepKeyOf ?? defaultStepKey;
  final idx = <String, int>{};
  for (var i = 0; i < stepOrder.length; i++) {
    idx[stepOrder[i]] = i;
  }
  final hidden = <String>{...?stepHidden};

  // Process candidate sources in natural step order — a field inside a
  // step hidden by an earlier-firing jump can never fire its own.
  final decoded = <Map<String, dynamic>>[];
  for (final raw0 in fields) {
    if (raw0 is Map) decoded.add(raw0.cast<String, dynamic>());
  }
  decoded.sort((a, b) => (idx[keyOf(a)] ?? -1).compareTo(idx[keyOf(b)] ?? -1));

  for (final raw in decoded) {
    final k = raw['key'];
    final fKey = k is String ? k : '';
    final cfg = raw['config'];
    final config = cfg is Map ? cfg.cast<String, dynamic>() : null;
    if (hidden.contains(keyOf(raw))) continue;
    if (!fieldVisible(config, answers, fKey)) continue;
    final j = config?['jump'];
    if (j is! Map) continue;
    final target = j['step'];
    if (target is! String || target.isEmpty) continue;
    final when = j['when'];
    final hasWhen = when is Map &&
        ((when['all'] is List && (when['all'] as List).isNotEmpty) ||
            (when['any'] is List && (when['any'] as List).isNotEmpty));
    if (hasWhen) {
      if (!evalCondition(when.cast<String, dynamic>(), answers, fKey)) continue;
    } else if (!_isAnswered(answers[fKey], answers.containsKey(fKey))) {
      continue;
    }
    final s = idx[keyOf(raw)];
    final t = idx[target];
    if (s == null || t == null || t <= s) continue;
    for (var i = s + 1; i < t; i++) {
      hidden.add(stepOrder[i]);
    }
  }
  return hidden;
}
