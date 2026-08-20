import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:japaconnect_core/japaconnect_core.dart';
import 'package:japaconnect_mobile/features/jobs/services/jobs_service.dart';

final jobsServiceProvider = Provider<JobsService>((ref) {
  final client = ref.watch(jcApiClientProvider);
  return JobsService(client);
});

final jobsListProvider =
    FutureProvider.autoDispose<List<Job>>((ref) async {
  final service = ref.watch(jobsServiceProvider);
  return service.listJobs();
});

final jobDetailProvider = FutureProvider.autoDispose
    .family<Job, String>((ref, slug) async {
  final service = ref.watch(jobsServiceProvider);
  return service.getJob(slug);
});
