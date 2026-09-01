import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/tools/readiness/providers/readiness_provider.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';

// Ref: japaconnect-engineering/sections/71-readiness-assessment.md
/// Mobile Readiness Assessment screen.
///
/// Loads the active form, supports multi-step (grouped by dimension),
/// submits answers, polls for the scored result, displays the result + roadmap,
/// and shows the authenticated user's history.
class ReadinessScreen extends ConsumerStatefulWidget {
  const ReadinessScreen({super.key});

  @override
  ConsumerState<ReadinessScreen> createState() => _ReadinessScreenState();
}

class _ReadinessScreenState extends ConsumerState<ReadinessScreen> {
  final Map<String, dynamic> _answers = {};
  int _currentStep = 0;
  bool _submitting = false;
  String? _submissionId;
  Map<String, dynamic>? _result;
  List<dynamic> _roadmap = [];
  Map<String, dynamic>? _journeyContext;
  bool _roadmapLoading = false;
  List<dynamic> _countryOptions = [];
  Map<String, dynamic> _journeyOptions = {};
  String? _error;
  Timer? _pollTimer;
  int _pollCount = 0;
  bool _historySelected = false;

  @override
  void dispose() {
    _pollTimer?.cancel();
    super.dispose();
  }

  void _reset() {
    setState(() {
      _answers.clear();
      _currentStep = 0;
      _submitting = false;
      _submissionId = null;
      _result = null;
      _roadmap = [];
      _journeyContext = null;
      _roadmapLoading = false;
      _error = null;
      _pollCount = 0;
      _historySelected = false;
    });
    _pollTimer?.cancel();
    ref.invalidate(readinessProvider);
    ref.invalidate(readinessHistoryProvider);
  }

  List<String> _stepOrder(List<dynamic> fields) {
    final seen = <String>[];
    for (final f in fields) {
      final config = (f['config'] as Map<String, dynamic>?) ?? {};
      final dim = (config['dimension'] as String?)?.toLowerCase() ?? 'general';
      if (!seen.contains(dim)) seen.add(dim);
    }
    return seen;
  }

  List<dynamic> _fieldsForStep(List<dynamic> fields, String step) {
    return fields.where((f) {
      final config = (f['config'] as Map<String, dynamic>?) ?? {};
      final dim = (config['dimension'] as String?)?.toLowerCase() ?? 'general';
      return dim == step;
    }).toList();
  }

  bool _isFieldRequired(dynamic field) {
    final f = field as Map<String, dynamic>?;
    if (f == null) return false;
    return f['is_required'] == true;
  }

  bool _isStepComplete(List<dynamic> stepFields) {
    for (final f in stepFields) {
      final field = f as Map<String, dynamic>?;
      if (field == null) continue;
      final key = field['key'] as String?;
      if (key == null) continue;
      if (!_isFieldRequired(field)) continue;
      final value = _answers[key];
      if (value == null) return false;
      if (value is String && value.isEmpty) return false;
      if (value is List && value.isEmpty) return false;
    }
    return true;
  }

  void _onFieldChanged(String key, dynamic value) {
    setState(() {
      _answers[key] = value;
      _error = null;
    });
  }

  Future<void> _submit(List<dynamic> fields) async {
    final required = fields.where(_isFieldRequired).toList();
    for (final f in required) {
      final field = f as Map<String, dynamic>?;
      if (field == null) continue;
      final key = field['key'] as String?;
      if (key == null) continue;
      final value = _answers[key];
      if (value == null || (value is String && value.isEmpty) || (value is List && value.isEmpty)) {
        setState(() => _error = 'Please answer all required questions.');
        return;
      }
    }

    setState(() {
      _submitting = true;
      _error = null;
      _result = null;
    });

    try {
      final service = ref.read(readinessServiceProvider);
      final response = await service.submit(Map.from(_answers));
      final submissionId = response['submission_id'] as String?;
      if (submissionId == null) throw Exception('No submission_id returned');

      setState(() {
        _submitting = false;
        _submissionId = submissionId;
      });
      _startPolling();
    } catch (e) {
      setState(() {
        _submitting = false;
        _error = 'Submission failed: $e';
      });
    }
  }

  void _startPolling() {
    _pollTimer?.cancel();
    _pollCount = 0;
    final service = ref.read(readinessServiceProvider);
    _pollTimer = Timer.periodic(const Duration(milliseconds: 1500), (timer) async {
      if (_submissionId == null || !mounted) {
        timer.cancel();
        return;
      }
      if (_pollCount >= 40) {
        timer.cancel();
        setState(() => _error = 'Scoring is taking longer than expected. Please try again later.');
        return;
      }
      _pollCount++;
      try {
        final data = await service.getResultBySubmission(_submissionId!);
        if (data.containsKey('id')) {
          timer.cancel();
          setState(() => _result = data);
          final resultId = data['id'] as String?;
          if (resultId != null) {
            await _loadRoadmap(resultId);
          }
          ref.invalidate(readinessHistoryProvider);
        }
      } catch (e) {
        // Best-effort polling; keep trying until max attempts.
      }
    });
  }

  Future<void> _loadResultFromHistory(String resultId) async {
    setState(() {
      _submitting = true;
      _error = null;
      _result = null;
      _roadmap = [];
      _journeyContext = null;
      _historySelected = true;
    });
    try {
      final service = ref.read(readinessServiceProvider);
      final data = await service.getResult(resultId);
      setState(() {
        _submitting = false;
        _result = data;
      });
      await _loadRoadmap(resultId);
    } catch (e) {
      setState(() {
        _submitting = false;
        _error = 'Failed to load result: $e';
      });
    }
  }

  Future<void> _loadRoadmap(String resultId) async {
    setState(() => _roadmapLoading = true);
    try {
      final service = ref.read(readinessServiceProvider);
      final data = await service.getRoadmap(resultId);
      setState(() {
        _roadmap = (data['items'] as List<dynamic>?) ?? [];
        _journeyContext = data['context'] as Map<String, dynamic>?;
        _roadmapLoading = false;
      });
    } catch (e) {
      setState(() => _roadmapLoading = false);
    }
  }

  Widget _buildSelectInput(String key, List<Map<String, dynamic>> options, dynamic value) {
    final groupValue = value as String?;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: options.map((opt) {
        final optValue = opt['value'] as String? ?? '';
        final optLabel = opt['label'] as String? ?? optValue;
        final selected = groupValue == optValue;
        return ListTile(
          leading: Icon(
            selected ? Icons.radio_button_checked : Icons.radio_button_unchecked,
          ),
          title: Text(optLabel),
          onTap: () => _onFieldChanged(key, optValue),
        );
      }).toList(),
    );
  }

  List<Map<String, dynamic>> _resolveFieldOptions(Map<String, dynamic> field) {
    final key = field['key'] as String? ?? '';

    if (key == 'destination_country' || key == 'origin_country') {
      return _countryOptions
          .whereType<Map<String, dynamic>>()
          .where((s) => s['geo_code'] != null && s['name'] != null)
          .map((s) => <String, dynamic>{
                'value': s['geo_code'] as String,
                'label': s['name'] as String,
              })
          .toList();
    }

    if (key == 'journey_stage') {
      final stages = _journeyOptions['stages'] as List<dynamic>?;
      if (stages != null) {
        return stages.whereType<Map<String, dynamic>>().toList();
      }
    } else if (key == 'visa_type') {
      final routes = _journeyOptions['visa_routes'] as List<dynamic>?;
      if (routes != null) {
        return routes.whereType<Map<String, dynamic>>().toList();
      }
    } else if (key == 'purpose') {
      final purposes = _journeyOptions['purposes'] as List<dynamic>?;
      if (purposes != null) {
        return purposes.whereType<Map<String, dynamic>>().toList();
      }
    }

    return (field['options'] as List<dynamic>? ?? []).cast<Map<String, dynamic>>();
  }

  Widget _buildFieldInput(Map<String, dynamic> field) {
    final key = field['key'] as String? ?? '';
    final fieldType = field['field_type'] as String? ?? 'text';
    final label = field['label'] as String? ?? '';
    final options = _resolveFieldOptions(field);
    final value = _answers[key];

    switch (fieldType) {
      case 'select':
        return _buildSelectInput(key, options, value);
      case 'multiselect':
        final selected = (value as List<dynamic>? ?? []).cast<String>().toSet();
        return Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: options.map((opt) {
            final optValue = opt['value'] as String? ?? '';
            final optLabel = opt['label'] as String? ?? optValue;
            return CheckboxListTile(
              title: Text(optLabel),
              value: selected.contains(optValue),
              onChanged: (checked) {
                if (checked == true) {
                  selected.add(optValue);
                } else {
                  selected.remove(optValue);
                }
                _onFieldChanged(key, selected.toList());
              },
            );
          }).toList(),
        );
      case 'number':
      case 'rating':
        final numberValue = (value as num?) ?? 0;
        final config = (field['config'] as Map<String, dynamic>?) ?? {};
        final min = (config['min'] as num?) ?? 0;
        final max = (config['max'] as num?) ?? 100;
        return Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(numberValue.toStringAsFixed(0)),
            Slider(
              value: numberValue.clamp(min.toDouble(), max.toDouble()).toDouble(),
              min: min.toDouble(),
              max: max.toDouble(),
              divisions: (max - min).toInt(),
              label: numberValue.toStringAsFixed(0),
              onChanged: (v) => _onFieldChanged(key, v.round()),
            ),
          ],
        );
      case 'boolean':
        return CheckboxListTile(
          title: Text(label),
          value: value == true || value == 1 || value == 'true' || value == 'yes',
          onChanged: (checked) => _onFieldChanged(key, checked == true),
        );
      case 'text':
      default:
        return TextField(
          decoration: InputDecoration(
            hintText: field['placeholder'] as String?,
            border: const OutlineInputBorder(),
          ),
          onChanged: (v) => _onFieldChanged(key, v),
          maxLines: null,
        );
    }
  }

  Widget _buildStepCard(Map<String, dynamic> form) {
    final fields = (form['fields'] as List<dynamic>? ?? []).cast<Map<String, dynamic>>();
    final order = _stepOrder(fields);
    if (order.isEmpty) {
      return const Center(child: Text('No questions available.'));
    }
    final step = order[_currentStep];
    final stepFields = _fieldsForStep(fields, step);
    final stepLabel = step[0].toUpperCase() + step.substring(1);

    return Padding(
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text(
            stepLabel,
            style: const TextStyle(fontSize: 18, fontWeight: FontWeight.bold),
          ),
          Text('Step ${_currentStep + 1} of ${order.length}'),
          const SizedBox(height: 16),
          ...stepFields.map((f) {
            final field = f as Map<String, dynamic>;
            return Padding(
              padding: const EdgeInsets.only(bottom: 16),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    '${field['label']}${_isFieldRequired(field) ? ' *' : ''}',
                    style: const TextStyle(fontWeight: FontWeight.w600),
                  ),
                  const SizedBox(height: 8),
                  _buildFieldInput(field),
                ],
              ),
            );
          }),
          const SizedBox(height: 24),
          Row(
            children: [
              if (_currentStep > 0)
                Expanded(
                  child: JcButton(
                    label: 'Back',
                    onPressed: () => setState(() => _currentStep--),
                  ),
                ),
              if (_currentStep > 0) const SizedBox(width: 12),
              if (_currentStep < order.length - 1)
                Expanded(
                  child: JcButton(
                    label: 'Next',
                    onPressed: _isStepComplete(stepFields)
                        ? () => setState(() => _currentStep++)
                        : null,
                  ),
                ),
              if (_currentStep == order.length - 1)
                Expanded(
                  child: JcButton(
                    label: 'Submit & get score',
                    onPressed: _isStepComplete(stepFields)
                        ? () => _submit(fields)
                        : null,
                  ),
                ),
            ],
          ),
        ],
      ),
    );
  }

  Widget _buildResultView() {
    final result = _result;
    if (result == null) return const SizedBox.shrink();

    final totalScore = result['total_score'] as num? ?? 0;
    final threshold = result['threshold_label'] as String? ?? '';
    final dimensions = (result['dimension_scores'] as Map<String, dynamic>?) ?? {};
    final fallbackRoadmap = (result['roadmap'] as List<dynamic>? ?? []).cast<Map<String, dynamic>>();
    final roadmap = _roadmap.isNotEmpty ? _roadmap.cast<Map<String, dynamic>>() : fallbackRoadmap;
    final auth = ref.watch(isAuthenticatedProvider);

    final destination = _journeyContext?['destination_country'] as String? ?? '';
    final origin = _journeyContext?['origin_country'] as String? ?? '';
    final stage = _journeyContext?['journey_stage'] as String? ?? '';
    final visa = _journeyContext?['visa_route'] as String? ?? '';
    final purpose = _journeyContext?['purpose'] as String? ?? '';
    final contextParts = [origin, destination, stage, visa, purpose].where((s) => s.isNotEmpty).toList();

    return Padding(
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text(
            'Your score',
            style: const TextStyle(fontSize: 14, fontWeight: FontWeight.w600),
            textAlign: TextAlign.center,
          ),
          Text(
            '$totalScore',
            style: const TextStyle(fontSize: 48, fontWeight: FontWeight.bold),
            textAlign: TextAlign.center,
          ),
          Text(
            threshold,
            style: const TextStyle(fontSize: 18),
            textAlign: TextAlign.center,
          ),
          const SizedBox(height: 16),
          const Text('Dimensions', style: TextStyle(fontWeight: FontWeight.bold)),
          const SizedBox(height: 8),
          ...dimensions.entries.map((e) {
            return ListTile(
              title: Text(e.key[0].toUpperCase() + e.key.substring(1)),
              trailing: Text('${e.value}'),
            );
          }),
          if (contextParts.isNotEmpty) ...[
            const SizedBox(height: 16),
            const Text('Your journey context', style: TextStyle(fontWeight: FontWeight.bold)),
            const SizedBox(height: 8),
            Wrap(
              spacing: 8,
              runSpacing: 8,
              children: contextParts.map((label) {
                return Chip(
                  label: Text(label),
                  backgroundColor: Theme.of(context).colorScheme.primaryContainer,
                );
              }).toList(),
            ),
          ],
          const SizedBox(height: 16),
          const Text('Your roadmap', style: TextStyle(fontWeight: FontWeight.bold)),
          const SizedBox(height: 8),
          if (_roadmapLoading) const LinearProgressIndicator(),
          ...roadmap.map((step) {
            return Card(
              margin: const EdgeInsets.only(bottom: 8),
              child: ListTile(
                title: Text(step['title'] as String? ?? ''),
                subtitle: Text(step['description'] as String? ?? ''),
              ),
            );
          }),
          if (!auth)
            const Padding(
              padding: EdgeInsets.only(top: 16),
              child: Text(
                'Sign in to save this result and view your history.',
                textAlign: TextAlign.center,
                style: TextStyle(fontStyle: FontStyle.italic),
              ),
            ),
          const SizedBox(height: 24),
          JcButton(
            label: 'Retake assessment',
            onPressed: _reset,
          ),
        ],
      ),
    );
  }

  Widget _buildHistory(AsyncValue<List<dynamic>> history) {
    return history.when(
      data: (items) {
        if (items.isEmpty) return const SizedBox.shrink();
        return Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Padding(
              padding: EdgeInsets.all(16),
              child: Text('History', style: TextStyle(fontSize: 18, fontWeight: FontWeight.bold)),
            ),
            ...items.map((h) {
              final entry = h as Map<String, dynamic>;
              final id = entry['id'] as String? ?? '';
              final label = entry['threshold_label'] as String? ?? '';
              final score = entry['total_score'] as num? ?? 0;
              return ListTile(
                title: Text('$label — $score%'),
                subtitle: Text(entry['created_at'] as String? ?? ''),
                onTap: () => _loadResultFromHistory(id),
              );
            }),
          ],
        );
      },
      loading: () => const SizedBox.shrink(),
      error: (e, stackTrace) => const SizedBox.shrink(),
    );
  }

  @override
  Widget build(BuildContext context) {
    final t = ref.watch(tProvider);
    final formState = ref.watch(readinessProvider);
    final history = ref.watch(readinessHistoryProvider);
    final journeyOptionsAsync = ref.watch(readinessJourneyOptionsProvider);
    final countryOptionsAsync = ref.watch(readinessCountryOptionsProvider);

    _journeyOptions = journeyOptionsAsync is AsyncData<Map<String, dynamic>>
        ? journeyOptionsAsync.value
        : {};
    _countryOptions = countryOptionsAsync is AsyncData<List<dynamic>>
        ? countryOptionsAsync.value
        : [];

    return Scaffold(
      appBar: AppBar(title: Text(t('tools.readiness'))),
      body: formState.when(
        data: (form) {
          if (_result != null || (_historySelected && _submitting)) {
            return SingleChildScrollView(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  if (_submitting) const LinearProgressIndicator(),
                  _buildResultView(),
                  _buildHistory(history),
                ],
              ),
            );
          }

          return SingleChildScrollView(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                if (_submitting || _pollTimer?.isActive == true) const LinearProgressIndicator(),
                _buildStepCard(form),
                if (_error != null)
                  Padding(
                    padding: const EdgeInsets.all(16),
                    child: Text(
                      _error!,
                      style: TextStyle(color: Theme.of(context).colorScheme.error),
                    ),
                  ),
                _buildHistory(history),
              ],
            ),
          );
        },
        loading: () => const Center(child: LoadingShimmer()),
        error: (e, stackTrace) => JcErrorWidget(
          error: e,
          onRetry: () => ref.invalidate(readinessProvider),
        ),
      ),
    );
  }
}
