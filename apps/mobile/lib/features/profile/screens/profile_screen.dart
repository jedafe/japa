import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/profile/providers/profile_provider.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';

class ProfileScreen extends ConsumerWidget {
  const ProfileScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(meProvider);
    final t = ref.watch(tProvider);
    return Scaffold(
      appBar: AppBar(title: Text(t('profile.title'))),
      body: state.when(
        data: (user) => ListView(
          children: [
            ListTile(
                title: Text(user['username'] ?? ''),
                subtitle: Text(user['email'] ?? '')),
            ListTile(
                title: Text(t('profile.settings')),
                onTap: () => context.push('/profile/settings')),
            ListTile(
                title: Text(t('profile.wallet')),
                onTap: () => context.push('/profile/wallet')),
            ListTile(
                title: Text(t('profile.bookmarks')),
                onTap: () => context.push('/profile/bookmarks')),
            ListTile(
              title: Text(t('profile.logout')),
              onTap: () async {
                await ref.read(authServiceProvider).logout();
                ref.read(isAuthenticatedProvider.notifier).set(false);
                if (context.mounted) context.go('/login');
              },
            ),
          ],
        ),
        loading: () => const LoadingShimmer(),
        error: (e, _) =>
            JcErrorWidget(error: e, onRetry: () => ref.invalidate(meProvider)),
      ),
    );
  }
}
