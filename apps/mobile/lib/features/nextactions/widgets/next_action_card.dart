import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../providers/next_actions_provider.dart';
import '../services/next_actions_service.dart';

/// §122 Next-Action card — renders the top suggestion for the mobile surface.
/// Hidden while loading or empty. CTA navigates in-app (relative CTA paths
/// only — the backend already validates CTA origin).
class NextActionCard extends ConsumerWidget {
  const NextActionCard({super.key});

  /// Channels the feed card can render — it must not claim suggestions
  /// meant for chrome the app doesn't have (popup/announcement_bar).
  static const _cardChannels = ['in_article', 'sidebar'];

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final async = ref.watch(nextActionsProvider);
    final list = async.value ?? const <NextAction>[];
    NextAction? found;
    for (final a in list) {
      if (a.channels.any(_cardChannels.contains)) {
        found = a;
        break;
      }
    }
    if (found == null) return const SizedBox.shrink();
    final action = found;

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
                      ref.read(nextActionsProvider.notifier).snooze(action),
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
