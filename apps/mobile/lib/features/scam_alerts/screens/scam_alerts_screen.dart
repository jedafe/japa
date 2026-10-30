// Ref: japaconnect-engineering/sections/81-scam-alert-board.md
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:japaconnect_mobile/features/scam_alerts/providers/scam_alerts_provider.dart';
import 'package:japaconnect_mobile/features/scam_alerts/services/scam_alerts_service.dart';
import 'package:japaconnect_mobile/i18n/app_localizations.dart';

class ScamAlertsScreen extends ConsumerWidget {
  const ScamAlertsScreen({super.key});

  String _typeLabel(String Function(String) t, String scamType) {
    switch (scamType) {
      case 'fraudulent_agent': return t('scam_alerts.type.fraudulent_agent');
      case 'fake_job': return t('scam_alerts.type.fake_job');
      case 'housing_fraud': return t('scam_alerts.type.housing_fraud');
      case 'fake_scholarship': return t('scam_alerts.type.fake_scholarship');
      case 'whatsapp_telegram': return t('scam_alerts.type.whatsapp_telegram');
      default: return t('scam_alerts.type.other');
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final t = ref.watch(tProvider);
    final alerts = ref.watch(scamAlertsProvider);
    return Scaffold(
      appBar: AppBar(title: Text(t('scam_alerts.title'))),
      floatingActionButton: FloatingActionButton.extended(
        onPressed: () => context.push('/scam-alerts/new'),
        icon: const Icon(Icons.report),
        label: Text(t('scam_alerts.report_button')),
      ),
      body: alerts.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => Center(
          child: TextButton(
            onPressed: () => ref.invalidate(scamAlertsProvider),
            child: Text(t('common.retry')),
          ),
        ),
        data: (items) => items.isEmpty
            ? Center(child: Text(t('scam_alerts.empty')))
            : RefreshIndicator(
                onRefresh: () async => ref.invalidate(scamAlertsProvider),
                child: ListView.separated(
                  itemCount: items.length,
                  separatorBuilder: (_, _) => const Divider(height: 1),
                  itemBuilder: (context, i) {
                    final a = items[i];
                    return ListTile(
                      leading: a.isPinned
                          ? const Icon(Icons.push_pin, color: Colors.red)
                          : const Icon(Icons.warning_amber),
                      title: Text(
                        '${_typeLabel(t, a.scamType)} · ${a.destinationSpaceName}',
                        style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 14),
                      ),
                      subtitle: Text(
                        a.description,
                        maxLines: 2,
                        overflow: TextOverflow.ellipsis,
                      ),
                      onTap: () => context.push('/scam-alerts/${a.id}'),
                    );
                  },
                ),
              ),
      ),
    );
  }
}

// Public so the detail screen can reuse the row shape.
typedef ScamAlert = ScamAlertItem;
