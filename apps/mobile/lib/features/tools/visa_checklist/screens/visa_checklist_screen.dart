import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/tools/visa_checklist/providers/visa_checklist_provider.dart';
import 'package:japaconnect_mobile/features/tools/visa_checklist/services/visa_checklist_service.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';

/// §70 visa checklist — origin → destination → visa type selector, then
/// the guide's document items with persisted progress (authenticated).
class VisaChecklistScreen extends ConsumerWidget {
  const VisaChecklistScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final selector = ref.watch(visaChecklistSelectorProvider);
    final sel = ref.watch(visaChecklistSelectionProvider);
    final guide = ref.watch(visaChecklistGuideProvider);
    final authed = ref.watch(isAuthenticatedProvider);
    final t = ref.watch(tProvider);

    return Scaffold(
      appBar: AppBar(title: Text(t('tools.visa_checklist'))),
      body: selector.when(
        data: (s) => _SelectorBody(
          selector: s,
          selection: sel,
          guide: guide,
          authed: authed,
        ),
        loading: () => const LoadingShimmer(),
        error: (e, _) => JcErrorWidget(
          error: e,
          onRetry: () => ref.invalidate(visaChecklistSelectorProvider),
        ),
      ),
    );
  }
}

class _SelectorBody extends ConsumerWidget {
  final ChecklistSelector selector;
  final VisaChecklistSelection selection;
  final AsyncValue<ChecklistGuide?> guide;
  final bool authed;

  const _SelectorBody({
    required this.selector,
    required this.selection,
    required this.guide,
    required this.authed,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final t = ref.watch(tProvider);
    final destinations = selector.destinations;
    final visaTypes = selection.destination?.visaTypes ?? const [];

    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        DropdownButtonFormField<String>(
          initialValue: selection.origin?.id,
          decoration: InputDecoration(
            labelText: t('checklist.step1'),
            border: const OutlineInputBorder(),
          ),
          items: selector.origins
              .map((o) => DropdownMenuItem(value: o.id, child: Text(o.name)))
              .toList(),
          onChanged: (id) {
            final o = selector.origins.where((x) => x.id == id).firstOrNull;
            ref.read(visaChecklistSelectionProvider.notifier).selectOrigin(o);
          },
        ),
        const SizedBox(height: 12),
        if (selection.origin != null) ...[
          DropdownButtonFormField<String>(
            initialValue: selection.destination?.id,
            decoration: InputDecoration(
              labelText: t('checklist.step2'),
              border: const OutlineInputBorder(),
            ),
            items: destinations
                .map((d) => DropdownMenuItem(value: d.id, child: Text(d.name)))
                .toList(),
            onChanged: (id) {
              final d = destinations.where((x) => x.id == id).firstOrNull;
              ref
                  .read(visaChecklistSelectionProvider.notifier)
                  .selectDestination(d);
            },
          ),
          const SizedBox(height: 12),
        ],
        if (selection.destination != null && visaTypes.isNotEmpty) ...[
          DropdownButtonFormField<String>(
            // Remount per destination — initialValue is read once, so a
            // destination switch would otherwise keep showing the old pick.
            key: ValueKey(selection.destination!.id),
            initialValue: selection.visaType?.value,
            decoration: InputDecoration(
              labelText: t('checklist.step3'),
              border: const OutlineInputBorder(),
            ),
            items: visaTypes
                .map(
                  (v) => DropdownMenuItem(value: v.value, child: Text(v.label)),
                )
                .toList(),
            onChanged: (v) {
              final vt = visaTypes.where((x) => x.value == v).firstOrNull;
              ref
                  .read(visaChecklistSelectionProvider.notifier)
                  .selectVisaType(vt);
            },
          ),
          const SizedBox(height: 12),
        ] else if (selection.destination != null) ...[
          Padding(
            padding: const EdgeInsets.symmetric(vertical: 8),
            child: Text(
              t('checklist.no_visa_types'),
              style: Theme.of(context).textTheme.bodySmall,
            ),
          ),
        ],
        if (selection.visaType != null) ...[
          const Divider(height: 24),
          guide.when(
            data: (g) => g == null
                ? const SizedBox.shrink()
                : _ChecklistItems(
                    guide: g,
                    authed: authed,
                    originSlug: selection.origin?.slug,
                    originCode: selection.origin?.code,
                  ),
            loading: () => const Padding(
              padding: EdgeInsets.symmetric(vertical: 24),
              child: Center(child: CircularProgressIndicator()),
            ),
            error: (e, _) => JcErrorWidget(
              error: e,
              onRetry: () => ref.invalidate(visaChecklistGuideProvider),
            ),
          ),
        ],
      ],
    );
  }
}

class _ChecklistItems extends ConsumerWidget {
  final ChecklistGuide guide;
  final bool authed;
  final String? originSlug;
  final String? originCode;

  const _ChecklistItems({
    required this.guide,
    required this.authed,
    this.originSlug,
    this.originCode,
  });

  Future<void> _toggle(
    WidgetRef ref,
    BuildContext context,
    ChecklistItem item,
    bool? value,
  ) async {
    final t = ref.read(tProvider);
    if (!authed) {
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text(t('checklist.anon_hint'))));
      return;
    }
    try {
      await ref
          .read(visaChecklistServiceProvider)
          .updateProgress(
            guide.guideId,
            item.itemKey,
            completed: value ?? false,
            origin: originSlug,
            originCode: originCode,
          );
      ref.invalidate(visaChecklistGuideProvider);
    } catch (e) {
      if (context.mounted) {
        ScaffoldMessenger.of(context)
            .showSnackBar(SnackBar(content: Text(t('checklist.load_error'))));
      }
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final t = ref.watch(tProvider);
    if (guide.items.isEmpty) {
      return Padding(
        padding: const EdgeInsets.symmetric(vertical: 16),
        child: Text(
          t('checklist.empty'),
          style: Theme.of(context).textTheme.bodySmall,
        ),
      );
    }

    final done = guide.items.where((i) => i.completed).length;
    final pct = done / guide.items.length;

    // Group items by the API-provided category (§70 contract); "General"
    // is the uncategorised fallback.
    final groups = <String, List<ChecklistItem>>{};
    for (final it in guide.items) {
      final cat = it.category.trim().isEmpty ? 'General' : it.category;
      groups.putIfAbsent(cat, () => []).add(it);
    }

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        if (guide.title.isNotEmpty)
          Padding(
            padding: const EdgeInsets.only(bottom: 8),
            child: Text(
              guide.title,
              style: Theme.of(context).textTheme.titleMedium,
            ),
          ),
        if (!authed)
          Card(
            margin: const EdgeInsets.only(bottom: 12),
            child: Padding(
              padding: const EdgeInsets.all(12),
              child: Text(
                t('checklist.anon_hint'),
                style: Theme.of(context).textTheme.bodySmall,
              ),
            ),
          ),
        Padding(
          padding: const EdgeInsets.only(bottom: 12),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                '${t('checklist.progress')} — $done/${guide.items.length} (${(pct * 100).round()}%)',
                style: Theme.of(context).textTheme.bodySmall,
              ),
              const SizedBox(height: 4),
              LinearProgressIndicator(value: pct),
            ],
          ),
        ),
        for (final entry in groups.entries) ...[
          if (groups.length > 1 || entry.key != 'General')
            Padding(
              padding: const EdgeInsets.only(top: 8, bottom: 4),
              child: Text(
                entry.key == 'General'
                    ? t('checklist.general_category')
                    : entry.key,
                style: Theme.of(context).textTheme.labelLarge
                    ?.copyWith(letterSpacing: 0.5),
              ),
            ),
          for (final item in entry.value)
            CheckboxListTile(
              contentPadding: EdgeInsets.zero,
              value: item.completed,
              onChanged: (v) => _toggle(ref, context, item, v),
              title: Text(item.label),
              subtitle: (item.description.isNotEmpty || item.required)
                  ? Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        if (item.required)
                          Text(
                            t('checklist.required'),
                            style: TextStyle(
                              fontSize: 11,
                              color: Theme.of(context).colorScheme.error,
                            ),
                          ),
                        if (item.description.isNotEmpty)
                          Text(
                            item.description,
                            style: Theme.of(context).textTheme.bodySmall,
                          ),
                        if (item.notes.isNotEmpty)
                          Text(
                            item.notes,
                            style: Theme.of(context).textTheme.bodySmall,
                          ),
                      ],
                    )
                  : null,
            ),
        ],
      ],
    );
  }
}
