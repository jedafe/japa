import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/profile/providers/profile_provider.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';
import 'package:japaconnect_mobile/features/social/screens/blocked_users_screen.dart';

class SettingsScreen extends ConsumerWidget {
  const SettingsScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(preferencesProvider);
    final t = ref.watch(tProvider);
    return Scaffold(
      appBar: AppBar(title: Text(t('profile.settings'))),
      body: state.when(
        data: (prefs) => ListView(
          children: [
            ListTile(
                title: Text(t('profile.theme')),
                subtitle: Text(prefs.theme ?? 'system')),
            ListTile(
                title: Text(t('profile.language')),
                subtitle: Text(prefs.language ?? 'en')),
            ListTile(
              title: Text(t('profile.blocked_users')),
              trailing: const Icon(Icons.chevron_right),
              onTap: () => Navigator.of(context).push(
                MaterialPageRoute<void>(
                    builder: (_) => const BlockedUsersScreen()),
              ),
            ),
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
