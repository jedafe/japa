// Ref: sections/77-flutter-mobile-app.md — draft manager for the mobile
// composer (BUG-CE-023). Merges local secure-storage drafts with synced
// /me/form-drafts entries; tap resumes in /compose, swipe deletes.
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:japaconnect_mobile/features/compose/providers/compose_provider.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';

class MyDraftsScreen extends ConsumerWidget {
  const MyDraftsScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final t = ref.watch(tProvider);
    final local = ref.watch(localDraftsProvider);
    final server = ref.watch(serverDraftsProvider);

    return Scaffold(
      appBar: AppBar(title: Text(t('compose.my_drafts'))),
      body: local.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => Center(child: Text(t('compose.load_failed'))),
        data: (localList) {
          final serverList = server.asData?.value ?? const [];
          final serverKeys = serverList.map((d) => d.key).toSet();
          // Local drafts whose key also exists server-side count once —
          // the synced blob shadows the local copy on other devices.
          final merged = [
            ...localList.map((d) => _DraftRow(
                key: d.key,
                title: d.title.isEmpty ? t('compose.untitled') : d.title,
                subtitle: d.contentType,
                synced: serverKeys.contains(d.key))),
            ...serverList
                .where((d) => !localList.any((l) => l.key == d.key))
                .map((d) => _DraftRow(
                    key: d.key,
                    title: (d.data['title']?.toString().isNotEmpty ?? false)
                        ? d.data['title'].toString()
                        : t('compose.untitled'),
                    subtitle: d.data['content_type']?.toString() ?? '',
                    synced: true)),
          ];
          if (merged.isEmpty) {
            return Center(child: Text(t('compose.no_drafts')));
          }
          return ListView.builder(
            itemCount: merged.length,
            itemBuilder: (ctx, i) {
              final d = merged[i];
              return Dismissible(
                key: ValueKey(d.key),
                direction: DismissDirection.endToStart,
                background: Container(color: Colors.red, alignment: Alignment.centerRight, padding: const EdgeInsets.only(right: 16), child: const Icon(Icons.delete, color: Colors.white)),
                onDismissed: (_) async {
                  await ref.read(localDraftStoreProvider).delete(d.key);
                  try {
                    await ref.read(composeServiceProvider).deleteServerDraft(d.key);
                  } catch (_) {}
                  ref.invalidate(localDraftsProvider);
                  ref.invalidate(serverDraftsProvider);
                },
                child: ListTile(
                  title: Text(d.title, maxLines: 1, overflow: TextOverflow.ellipsis),
                  subtitle: Text(d.synced ? '${d.subtitle} · ${t('compose.synced')}' : d.subtitle),
                  onTap: () => context.push('/compose?draft=${d.key}'),
                ),
              );
            },
          );
        },
      ),
    );
  }
}

class _DraftRow {
  final String key;
  final String title;
  final String subtitle;
  final bool synced;
  const _DraftRow({required this.key, required this.title, required this.subtitle, required this.synced});
}
