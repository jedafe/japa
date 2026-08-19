import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/learning/providers/learning_provider.dart';

class CourseDetailScreen extends ConsumerWidget {
  final String slug;
  const CourseDetailScreen({super.key, required this.slug});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(courseDetailProvider(slug));
    return Scaffold(
      appBar: AppBar(title: const Text('Course')),
      body: state.when(
        data: (course) => SingleChildScrollView(
          padding: const EdgeInsets.all(16),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(course['title'] ?? '', style: JcTypography.heading),
              const SizedBox(height: 8),
              Text(course['description'] ?? ''),
              JcButton(
                label: 'Enroll',
                onPressed: () => ref
                    .read(learningServiceProvider)
                    .enroll(course['id'] as String),
              ),
            ],
          ),
        ),
        loading: () => const LoadingShimmer(),
        error: (e, _) => JcErrorWidget(
          error: e,
          onRetry: () => ref.invalidate(courseDetailProvider(slug)),
        ),
      ),
    );
  }
}
