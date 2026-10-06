import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/academy/providers/academy_provider.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';

class LessonPlayerScreen extends ConsumerWidget {
  final String trackSlug;
  final String moduleSlug;
  final String lessonSlug;
  const LessonPlayerScreen({
    super.key,
    required this.trackSlug,
    required this.moduleSlug,
    required this.lessonSlug,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(
      courseLessonProvider((trackSlug, moduleSlug, lessonSlug)),
    );
    final t = ref.watch(tProvider);
    return Scaffold(
      appBar: AppBar(title: Text(t('academy.lesson'))),
      body: state.when(
        data: (lesson) => SingleChildScrollView(
          padding: const EdgeInsets.all(16),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(lesson.title, style: JcTypography.heading),
              const SizedBox(height: 8),
              Text(lesson.body ?? ''),
              JcButton(
                label: t('academy.mark_complete'),
                onPressed: () => ref
                    .read(academyServiceProvider)
                    .updateProgress(lesson.id, 1.0),
              ),
            ],
          ),
        ),
        loading: () => const LoadingShimmer(),
        error: (e, _) => JcErrorWidget(
          error: e,
          onRetry: () => ref.invalidate(
            courseLessonProvider((trackSlug, moduleSlug, lessonSlug)),
          ),
        ),
      ),
    );
  }
}
