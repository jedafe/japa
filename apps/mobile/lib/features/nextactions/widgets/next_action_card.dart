import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../providers/next_actions_provider.dart';

/// §122 Next-Action card — renders the top suggestion for the mobile surface.
/// Hidden while loading or empty. CTA navigates in-app (relative CTA paths
/// only — the backend already validates CTA origin).
class NextActionCard extends ConsumerWidget {
  const NextActionCard({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final async = ref.watch(nextActionsProvider);
    final action = async.value?.isNotEmpty == true ? async.value!.first : null;
    if (action == null) return const SizedBox.shrink();

    final theme = Theme.of(context);
    return Card(
      margin: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(action.title, style: theme.textTheme.titleSmall),
            if (action.body.isNotEmpty) ...[
              const SizedBox(height: 4),
              Text(action.body, style: theme.textTheme.bodySmall),
            ],
            const SizedBox(height: 12),
            Row(
              children: [
                FilledButton(
                  onPressed: () {
                    ref.read(nextActionsProvider.notifier).act(action);
                    if (action.ctaUrl.startsWith('/')) {
                      context.push(action.ctaUrl);
                    }
                  },
                  child: Text(action.ctaLabel),
                ),
                TextButton(
                  onPressed: () =>
                      ref.read(nextActionsProvider.notifier).dismiss(action),
                  child: const Text('Not now'),
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }
}
