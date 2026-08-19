import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/tools/hub/providers/tools_hub_provider.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';

class ToolsHubScreen extends ConsumerWidget {
  const ToolsHubScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(toolsHubProvider);
    final t = ref.watch(tProvider);
    return Scaffold(
      appBar: AppBar(title: Text(t('tools.title'))),
      body: state.when(
        data: (flags) {
          final items = <Widget>[
            if (flags['visa_checklist'] != false)
              ListTile(
                title: Text(t('tools.visa_checklist')),
                onTap: () => context.push('/tools/visa-checklist'),
              ),
            if (flags['readiness'] != false)
              ListTile(
                title: Text(t('tools.readiness')),
                onTap: () => context.push('/tools/readiness'),
              ),
            if (flags['cost_of_living'] != false)
              ListTile(
                title: Text(t('tools.cost_of_living')),
                onTap: () => context.push('/tools/cost-of-living'),
              ),
            if (flags['scholarships'] != false)
              ListTile(
                title: Text(t('tools.scholarships')),
                onTap: () => context.push('/tools/scholarships'),
              ),
            if (flags['remittance'] != false)
              ListTile(
                title: Text(t('tools.remittance')),
                onTap: () => context.push('/tools/remittance'),
              ),
            if (flags['japabot'] != false)
              ListTile(
                title: Text(t('tools.japabot')),
                onTap: () => context.push('/tools/japabot'),
              ),
          ];
          return ListView(children: items);
        },
        loading: () => const LoadingShimmer(),
        error: (e, _) => JcErrorWidget(
          error: e,
          onRetry: () => ref.invalidate(toolsHubProvider),
        ),
      ),
    );
  }
}
