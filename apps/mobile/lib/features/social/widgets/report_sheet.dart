import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_mobile/features/social/services/report_service.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';

/// Shared report bottom sheet (§51) — the mobile analogue of the web
/// ReportDialog. Reason codes come from moderation.report_reasons via
/// ReportService, notes is required when reason == 'other'.
Future<void> showReportSheet(
  BuildContext context, {
  required Future<void> Function(String reason, String? notes) onSubmit,
}) {
  return showModalBottomSheet<void>(
    context: context,
    isScrollControlled: true,
    builder: (ctx) => _ReportSheet(onSubmit: onSubmit),
  );
}

class _ReportSheet extends ConsumerStatefulWidget {
  final Future<void> Function(String reason, String? notes) onSubmit;
  const _ReportSheet({required this.onSubmit});

  @override
  ConsumerState<_ReportSheet> createState() => _ReportSheetState();
}

class _ReportSheetState extends ConsumerState<_ReportSheet> {
  final _notesController = TextEditingController();
  String? _reason;
  String? _error;
  bool _submitting = false;
  bool _submitted = false;
  List<String> _reasons = const [];

  @override
  void initState() {
    super.initState();
    ref.read(reportServiceProvider).reportReasons().then((r) {
      if (mounted) setState(() => _reasons = r);
    });
  }

  @override
  void dispose() {
    _notesController.dispose();
    super.dispose();
  }

  String _humanize(String code) =>
      code.replaceAll('_', ' ').replaceFirstMapped(RegExp(r'^\w'), (m) => m[0]!.toUpperCase());

  Future<void> _submit() async {
    final t = ref.read(tProvider);
    if (_reason == null) {
      setState(() => _error = t('report.reason_required'));
      return;
    }
    final notes = _notesController.text.trim();
    if (_reason == 'other' && notes.isEmpty) {
      setState(() => _error = t('report.other_notes_required'));
      return;
    }
    setState(() {
      _submitting = true;
      _error = null;
    });
    try {
      await widget.onSubmit(_reason!, notes.isEmpty ? null : notes);
      if (mounted) setState(() => _submitted = true);
    } catch (e) {
      if (mounted) {
        setState(() {
          _submitting = false;
          _error = e.toString();
        });
      }
      return;
    }
    if (mounted) setState(() => _submitting = false);
  }

  @override
  Widget build(BuildContext context) {
    final t = ref.watch(tProvider);
    return Padding(
      padding: EdgeInsets.only(
        left: 16,
        right: 16,
        top: 16,
        bottom: MediaQuery.of(context).viewInsets.bottom + 16,
      ),
      child: _submitted
          ? Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                Text(t('report.success')),
                const SizedBox(height: 12),
                FilledButton(
                  onPressed: () => Navigator.of(context).pop(),
                  child: Text(t('common.close')),
                ),
              ],
            )
          : Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Text(t('report.title'), style: Theme.of(context).textTheme.titleMedium),
                const SizedBox(height: 12),
                DropdownButtonFormField<String>(
                  initialValue: _reason,
                  decoration: InputDecoration(labelText: t('report.reason_label')),
                  items: _reasons
                      .map((r) => DropdownMenuItem(
                            value: r,
                            child: Text(t('report.reason.$r') == 'report.reason.$r' ? _humanize(r) : t('report.reason.$r')),
                          ))
                      .toList(),
                  onChanged: (v) => setState(() => _reason = v),
                ),
                const SizedBox(height: 12),
                TextField(
                  controller: _notesController,
                  maxLines: 4,
                  maxLength: 5000,
                  decoration: InputDecoration(
                    labelText: _reason == 'other'
                        ? '${t('report.notes_label')} *'
                        : t('report.notes_label'),
                    hintText: t('report.notes_placeholder'),
                  ),
                ),
                if (_error != null)
                  Padding(
                    padding: const EdgeInsets.only(top: 8),
                    child: Text(
                      _error!,
                      style: TextStyle(color: Theme.of(context).colorScheme.error),
                    ),
                  ),
                const SizedBox(height: 12),
                Row(
                  mainAxisAlignment: MainAxisAlignment.end,
                  children: [
                    TextButton(
                      onPressed: () => Navigator.of(context).pop(),
                      child: Text(t('common.cancel')),
                    ),
                    const SizedBox(width: 8),
                    FilledButton(
                      onPressed: _submitting || _reasons.isEmpty ? null : _submit,
                      child: _submitting
                          ? const SizedBox(
                              width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2))
                          : Text(t('report.submit')),
                    ),
                  ],
                ),
              ],
            ),
    );
  }
}
