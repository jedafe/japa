import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/academy/providers/academy_provider.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';

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
      await ref.read(academyServiceProvider).enroll(courseId);
      if (mounted) {
        final t = ref.read(tProvider);
        ScaffoldMessenger.of(context)
            .showSnackBar(SnackBar(content: Text(t('academy.enrolled'))));
      }
    } catch (e) {
      if (mounted) {
        final t = ref.read(tProvider);
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(t('errors.failed_enroll', {'error': e.toString()})),
          ),
        );
      }
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(courseDetailProvider(widget.slug));
    final t = ref.watch(tProvider);
    return Scaffold(
      appBar: AppBar(title: Text(t('academy.course'))),
      body: state.when(
        data: (course) => SingleChildScrollView(
          padding: const EdgeInsets.all(16),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(course.title, style: JcTypography.heading),
              const SizedBox(height: 8),
              Text(course.description ?? ''),
              JcButton(
                label: t('academy.enroll'),
                onPressed: _loading ? null : () => _enroll(course.id),
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
