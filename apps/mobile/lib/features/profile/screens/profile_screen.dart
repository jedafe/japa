import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/profile/providers/profile_provider.dart';

class ProfileScreen extends ConsumerWidget {
  const ProfileScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(meProvider);
    return Scaffold(
      appBar: AppBar(title: const Text('Profile')),
      body: state.when(
        data: (user) => ListView(
          children: [
            ListTile(
                title: Text(user['username'] ?? ''),
                subtitle: Text(user['email'] ?? '')),
            ListTile(
                title: const Text('Settings'),
                onTap: () => context.push('/profile/settings')),
            ListTile(
                title: const Text('Wallet'),
                onTap: () => context.push('/profile/wallet')),
            ListTile(
                title: const Text('Bookmarks'),
                onTap: () => context.push('/profile/bookmarks')),
            ListTile(
              title: const Text('Logout'),
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
