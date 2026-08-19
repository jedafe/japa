import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/tools/readiness/providers/readiness_provider.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';

class ReadinessScreen extends ConsumerWidget {
  const ReadinessScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(readinessProvider);
    final t = ref.watch(tProvider);
    return Scaffold(
      appBar: AppBar(title: Text(t('tools.readiness'))),
      body: state.when(
        data: (assessment) => ListView(
          children: (assessment['questions'] as List? ?? [])
              .map<Widget>((q) => ListTile(title: Text(q['text'] ?? '')))
              .toList(),
        ),
        loading: () => const LoadingShimmer(),
        error: (e, _) => JcErrorWidget(
          error: e,
          onRetry: () => ref.invalidate(readinessProvider),
        ),
      ),
    );
  }
}
