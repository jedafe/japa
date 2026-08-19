import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/jobs/providers/jobs_provider.dart';

class JobDetailScreen extends ConsumerWidget {
  final String slug;
  const JobDetailScreen({super.key, required this.slug});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(jobDetailProvider(slug));
    return Scaffold(
      appBar: AppBar(title: const Text('Job')),
      body: state.when(
        data: (job) => SingleChildScrollView(
          padding: const EdgeInsets.all(16),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(job['title'] ?? '', style: JcTypography.heading),
              const SizedBox(height: 8),
              Text(job['description'] ?? ''),
              JcButton(
                label: 'Apply',
                onPressed: () =>
                    ref.read(jobsServiceProvider).apply(job['id'] as String),
              ),
            ],
          ),
        ),
        loading: () => const LoadingShimmer(),
        error: (e, _) => JcErrorWidget(
          error: e,
          onRetry: () => ref.invalidate(jobDetailProvider(slug)),
        ),
      ),
    );
  }
}
