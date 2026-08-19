import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/content/providers/content_provider.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';

class GuideDetailScreen extends ConsumerWidget {
  final String slug;
  const GuideDetailScreen({super.key, required this.slug});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(guideDetailProvider(slug));
    final t = ref.watch(tProvider);
    return Scaffold(
      appBar: AppBar(title: Text(t('content.guide'))),
      body: state.when(
        data: (item) => SingleChildScrollView(
          padding: const EdgeInsets.all(16),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(item['title'] ?? '', style: JcTypography.heading),
              const SizedBox(height: 8),
              Text(item['body'] ?? ''),
            ],
          ),
        ),
        loading: () => const LoadingShimmer(),
        error: (e, _) => JcErrorWidget(
            error: e, onRetry: () => ref.invalidate(guideDetailProvider(slug))),
      ),
    );
  }
}
