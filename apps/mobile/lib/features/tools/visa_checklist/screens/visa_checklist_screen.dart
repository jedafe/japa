import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/tools/visa_checklist/providers/visa_checklist_provider.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';

class VisaChecklistScreen extends ConsumerWidget {
  const VisaChecklistScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(visaChecklistProvider);
    final t = ref.watch(tProvider);
    return Scaffold(
      appBar: AppBar(title: Text(t('tools.visa_checklist'))),
      body: state.when(
        data: (items) => ListView.builder(
          itemCount: items.length,
          itemBuilder: (_, i) => CheckboxListTile(
            title: Text(items[i]['label'] ?? ''),
            value: items[i]['completed'] ?? false,
            onChanged: (v) {
              final service = ref.read(visaChecklistServiceProvider);
              service.updateProgress(
                  items[i]['checklist_id'], items[i]['id'], v ?? false);
            },
          ),
        ),
        loading: () => const LoadingShimmer(),
        error: (e, _) => JcErrorWidget(
          error: e,
          onRetry: () => ref.invalidate(visaChecklistProvider),
        ),
      ),
    );
  }
}
