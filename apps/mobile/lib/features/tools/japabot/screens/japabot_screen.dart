import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/tools/japabot/providers/japabot_provider.dart';

class JapaBotScreen extends ConsumerWidget {
  const JapaBotScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(japabotConversationsProvider);
    return Scaffold(
      appBar: AppBar(title: const Text('JapaBot')),
      body: state.when(
        data: (items) => ListView.builder(
          itemCount: items.length,
          itemBuilder: (_, i) =>
              ListTile(title: Text(items[i]['title'] ?? 'Conversation')),
        ),
        loading: () => const LoadingShimmer(),
        error: (e, _) => JcErrorWidget(
          error: e,
          onRetry: () => ref.invalidate(japabotConversationsProvider),
        ),
      ),
    );
  }
}
