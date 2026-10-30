// Ref: japaconnect-engineering/sections/81-scam-alert-board.md
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/scam_alerts/providers/scam_alerts_provider.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';

class ReportScamScreen extends ConsumerStatefulWidget {
  const ReportScamScreen({super.key});

  @override
  ConsumerState<ReportScamScreen> createState() => _ReportScamScreenState();
}

class _ReportScamScreenState extends ConsumerState<ReportScamScreen> {
  final _descCtrl = TextEditingController();
  String? _destId;
  String? _scamType;
  String? _platform;
  bool _authorities = false;
  bool _sending = false;

  @override
  void dispose() {
    _descCtrl.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    final t = ref.read(tProvider);
    if (_destId == null || _scamType == null || _descCtrl.text.trim().length < 20) {
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text(t('scam_alerts.form.incomplete'))));
      return;
    }
    setState(() => _sending = true);
    final body = <String, dynamic>{
      'scam_type': _scamType,
      'destination_space_id': _destId,
      'description': _descCtrl.text.trim(),
      'reported_to_authorities': _authorities,
    };
    if (_platform != null && _platform!.isNotEmpty) body['platform_used'] = _platform;
    try {
      await ref.read(scamAlertsServiceProvider).submit(body);
      if (!mounted) return;
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text(t('scam_alerts.form.success'))));
      context.pop();
    } catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(t('scam_alerts.form.failed'))));
    } finally {
      if (mounted) setState(() => _sending = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final t = ref.watch(tProvider);
    final countries = ref.watch(countriesProvider);
    final schema = ref.watch(scamAlertSchemaProvider);
    return Scaffold(
      appBar: AppBar(title: Text(t('scam_alerts.report_button'))),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          countries.when(
            loading: () => const LinearProgressIndicator(),
            error: (e, _) => Text(t('common.retry')),
            data: (list) => DropdownButtonFormField<String>(
              initialValue: _destId,
              decoration: InputDecoration(labelText: t('scam_alerts.form.destination')),
              items: [
                for (final c in list)
                  DropdownMenuItem(value: c.id, child: Text(c.name)),
              ],
              onChanged: (v) => setState(() => _destId = v),
            ),
          ),
          const SizedBox(height: 12),
          schema.when(
            loading: () => const LinearProgressIndicator(),
            error: (e, _) => const SizedBox.shrink(),
            data: (opts) => Column(children: [
              DropdownButtonFormField<String>(
                initialValue: _scamType,
                decoration: InputDecoration(labelText: t('scam_alerts.form.type')),
                items: [
                  for (final o in opts['scam_types'] ?? [])
                    DropdownMenuItem(value: o['value'], child: Text(o['label']!)),
                ],
                onChanged: (v) => setState(() => _scamType = v),
              ),
              const SizedBox(height: 12),
              DropdownButtonFormField<String>(
                initialValue: _platform,
                decoration: InputDecoration(labelText: t('scam_alerts.form.platform')),
                items: [
                  for (final o in opts['platforms'] ?? [])
                    DropdownMenuItem(value: o['value'], child: Text(o['label']!)),
                ],
                onChanged: (v) => setState(() => _platform = v),
              ),
            ]),
          ),
          const SizedBox(height: 12),
          TextField(
            controller: _descCtrl,
            maxLines: 6,
            decoration: InputDecoration(
              labelText: t('scam_alerts.form.description'),
              hintText: t('scam_alerts.form.description_hint'),
              alignLabelWithHint: true,
            ),
          ),
          const SizedBox(height: 8),
          CheckboxListTile(
            value: _authorities,
            onChanged: (v) => setState(() => _authorities = v ?? false),
            title: Text(t('scam_alerts.form.authorities')),
            controlAffinity: ListTileControlAffinity.leading,
            contentPadding: EdgeInsets.zero,
          ),
          const SizedBox(height: 12),
          FilledButton(
            onPressed: _sending ? null : _submit,
            child: _sending
                ? const SizedBox(height: 20, width: 20, child: CircularProgressIndicator(strokeWidth: 2))
                : Text(t('scam_alerts.form.submit')),
          ),
        ],
      ),
    );
  }
}
