import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/profile/providers/profile_provider.dart';

class SettingsScreen extends ConsumerWidget {
  const SettingsScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(preferencesProvider);
    return Scaffold(
      appBar: AppBar(title: const Text('Settings')),
      body: state.when(
        data: (prefs) => ListView(
          children: [
            ListTile(
                title: const Text('Theme'),
                subtitle: Text(prefs['theme'] ?? 'system')),
            ListTile(
                title: const Text('Language'),
                subtitle: Text(prefs['language'] ?? 'en')),
          ],
        ),
        loading: () => const LoadingShimmer(),
        error: (e, _) => JcErrorWidget(
            error: e,
            onRetry: () => ref.invalidate(preferencesProvider)),
      ),
    );
  }
}
