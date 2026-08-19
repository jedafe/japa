import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/jobs/providers/jobs_provider.dart';

class JobDetailScreen extends ConsumerStatefulWidget {
  final String slug;
  const JobDetailScreen({super.key, required this.slug});

  @override
  ConsumerState<JobDetailScreen> createState() => _JobDetailScreenState();
}

class _JobDetailScreenState extends ConsumerState<JobDetailScreen> {
  bool _loading = false;

  Future<void> _apply(String jobId) async {
    setState(() => _loading = true);
    try {
      await ref.read(jobsServiceProvider).apply(jobId);
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('Applied!')),
        );
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('Failed to apply: $e')),
        );
      }
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(jobDetailProvider(widget.slug));
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
                onPressed:
                    _loading ? null : () => _apply(job['id'] as String),
              ),
            ],
          ),
        ),
        loading: () => const LoadingShimmer(),
        error: (e, _) => JcErrorWidget(
          error: e,
          onRetry: () => ref.invalidate(jobDetailProvider(widget.slug)),
        ),
      ),
    );
  }
}
