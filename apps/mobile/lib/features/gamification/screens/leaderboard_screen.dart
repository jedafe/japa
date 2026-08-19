import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/gamification/providers/gamification_provider.dart';

class LeaderboardScreen extends ConsumerWidget {
  const LeaderboardScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(leaderboardProvider);
    return Scaffold(
      appBar: AppBar(title: const Text('Leaderboard')),
      body: state.when(
        data: (leaderboard) => ListView.builder(
          itemCount: (leaderboard['entries'] as List? ?? []).length,
          itemBuilder: (_, i) {
            final entry = (leaderboard['entries'] as List<dynamic>)[i] as Map<String, dynamic>;
            return ListTile(
              title: Text(entry['username'] ?? ''),
              trailing: Text(entry['points']?.toString() ?? '0'),
            );
          },
        ),
        loading: () => const LoadingShimmer(),
        error: (e, _) => JcErrorWidget(error: e, onRetry: () => ref.invalidate(leaderboardProvider)),
      ),
    );
  }
}
