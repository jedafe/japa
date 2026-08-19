import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/profile/providers/profile_provider.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';

class WalletScreen extends ConsumerWidget {
  const WalletScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(walletProvider);
    final t = ref.watch(tProvider);
    return Scaffold(
      appBar: AppBar(title: Text(t('profile.wallet'))),
      body: state.when(
        data: (wallet) => ListView(
          children: [
            ListTile(
                title: Text(t('profile.balance')),
                subtitle:
                    Text(wallet['balance']?.toString() ?? '0')),
          ],
        ),
        loading: () => const LoadingShimmer(),
        error: (e, _) => JcErrorWidget(
            error: e,
            onRetry: () => ref.invalidate(walletProvider)),
      ),
    );
  }
}
