import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/profile/providers/profile_provider.dart';

class WalletScreen extends ConsumerWidget {
  const WalletScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(walletProvider);
    return Scaffold(
      appBar: AppBar(title: const Text('Wallet')),
      body: state.when(
        data: (wallet) => ListView(
          children: [
            ListTile(
                title: const Text('Balance'),
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
