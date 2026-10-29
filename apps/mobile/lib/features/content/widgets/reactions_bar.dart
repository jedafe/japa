import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_mobile/features/content/providers/content_provider.dart';

/// Emoji reaction chips for content (§14) — mobile parity with the web
/// ReactionRail. Tap toggles the current user's reaction; long-press shows
/// who reacted.
class ReactionsBar extends ConsumerWidget {
  final String contentId;
  const ReactionsBar({super.key, required this.contentId});

  Future<void> _toggle(WidgetRef ref, String key, bool active) async {
    final svc = ref.read(reactionServiceProvider);
    try {
      if (active) {
        await svc.unreact(contentId, key);
      } else {
        await svc.react(contentId, key);
      }
    } finally {
      ref.invalidate(contentReactionsProvider(contentId));
    }
  }

  Future<void> _showReactors(BuildContext context, WidgetRef ref, String key) async {
    final reactors = await ref.read(reactionServiceProvider).getReactors(contentId, reactionKey: key);
    if (!context.mounted) return;
    showModalBottomSheet<void>(
      context: context,
      builder: (ctx) => SafeArea(
        child: reactors.isEmpty
            ? const Padding(padding: EdgeInsets.all(24), child: Text('No reactions yet'))
            : ListView.builder(
                shrinkWrap: true,
                itemCount: reactors.length,
                itemBuilder: (_, i) {
                  final u = reactors[i];
                  return ListTile(
                    dense: true,
                    title: Text(u.displayName.isNotEmpty ? u.displayName : u.username),
                  );
                },
              ),
      ),
    );
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(contentReactionsProvider(contentId));
    return state.when(
      data: (s) => Wrap(
        spacing: 8,
        children: [
          for (final t in s.types)
            ActionChip(
              avatar: Text(t.emoji),
              label: Text('${s.counts[t.key] ?? 0}'),
              tooltip: t.label,
              backgroundColor: s.mine.contains(t.key)
                  ? Theme.of(context).colorScheme.primaryContainer
                  : null,
              onPressed: () => _toggle(ref, t.key, s.mine.contains(t.key)),
              onLongPress: () => _showReactors(context, ref, t.key),
            ),
        ],
      ),
      loading: () => const SizedBox(height: 40),
      error: (_, __) => const SizedBox.shrink(),
    );
  }
}
