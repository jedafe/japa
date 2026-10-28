import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/social/providers/social_provider.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';

/// §41 — Blocked accounts management (BLK-019). Lists users the viewer has
/// blocked with an inline Unblock action; removal is optimistic with a
/// reload fallback on failure.
class BlockedUsersScreen extends ConsumerStatefulWidget {
  const BlockedUsersScreen({super.key});

  @override
  ConsumerState<BlockedUsersScreen> createState() => _BlockedUsersScreenState();
}

class _BlockedUsersScreenState extends ConsumerState<BlockedUsersScreen> {
  late Future<List<Map<String, dynamic>>> _future;
  final Set<String> _removing = {};

  @override
  void initState() {
    super.initState();
    _future = ref.read(socialServiceProvider).listBlockedUsers();
  }

  Future<void> _unblock(Map<String, dynamic> user) async {
    final id = user['blocked_id'] as String;
    if (_removing.contains(id)) return;
    setState(() => _removing.add(id));
    try {
      await ref.read(socialServiceProvider).unblockUser(id);
      setState(() {
        _future = ref.read(socialServiceProvider).listBlockedUsers();
      });
    } finally {
      if (mounted) setState(() => _removing.remove(id));
    }
  }

  @override
  Widget build(BuildContext context) {
    final t = ref.watch(tProvider);
    return Scaffold(
      appBar: AppBar(title: Text(t('profile.blocked_users'))),
      body: FutureBuilder<List<Map<String, dynamic>>>(
        future: _future,
        builder: (context, snap) {
          if (snap.hasError) {
            return JcErrorWidget(
              error: snap.error!,
              onRetry: () => setState(
                  () => _future = ref.read(socialServiceProvider).listBlockedUsers()),
            );
          }
          if (!snap.hasData) return const LoadingShimmer();
          final users = snap.data!;
          if (users.isEmpty) {
            return Center(child: Text(t('profile.blocked_users_empty')));
          }
          return ListView.builder(
            itemCount: users.length,
            itemBuilder: (context, i) {
              final u = users[i];
              final id = u['blocked_id'] as String;
              final name = (u['display_name'] as String?)?.isNotEmpty == true
                  ? u['display_name'] as String
                  : '@${u['username']}';
              return ListTile(
                leading: u['avatar_url'] != null && (u['avatar_url'] as String).isNotEmpty
                    ? CircleAvatar(backgroundImage: NetworkImage(u['avatar_url'] as String))
                    : const CircleAvatar(child: Icon(Icons.person)),
                title: Text(name),
                subtitle: Text('@${u['username'] ?? ''}'),
                trailing: TextButton(
                  onPressed: _removing.contains(id) ? null : () => _unblock(u),
                  child: Text(t('profile.unblock')),
                ),
              );
            },
          );
        },
      ),
    );
  }
}
