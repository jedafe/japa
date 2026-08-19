import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/learning/providers/learning_provider.dart';

class CourseDetailScreen extends ConsumerStatefulWidget {
  final String slug;
  const CourseDetailScreen({super.key, required this.slug});

  @override
  ConsumerState<CourseDetailScreen> createState() => _CourseDetailScreenState();
}

class _CourseDetailScreenState extends ConsumerState<CourseDetailScreen> {
  bool _loading = false;

  Future<void> _enroll(String courseId) async {
    setState(() => _loading = true);
    try {
      await ref.read(learningServiceProvider).enroll(courseId);
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('Enrolled!')),
        );
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('Failed to enroll: $e')),
        );
      }
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(courseDetailProvider(widget.slug));
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
                onPressed:
                    _loading ? null : () => _enroll(course['id'] as String),
              ),
            ],
          ),
        ),
        loading: () => const LoadingShimmer(),
        error: (e, _) => JcErrorWidget(
          error: e,
          onRetry: () => ref.invalidate(courseDetailProvider(widget.slug)),
        ),
      ),
    );
  }
}
