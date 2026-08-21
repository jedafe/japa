class JobApplication {
  final String id;
  final String jobId;
  final String? jobTitle;
  final String? jobSlug;
  final String status;
  final String? employerNotes;
  final DateTime? appliedAt;
  final DateTime? statusUpdatedAt;

  JobApplication({
    required this.id,
    required this.jobId,
    this.jobTitle,
    this.jobSlug,
    required this.status,
    this.employerNotes,
    this.appliedAt,
    this.statusUpdatedAt,
  });

  factory JobApplication.fromJson(Map<String, dynamic> json) {
    return JobApplication(
      id: json['id'] as String,
      jobId: json['job_id'] as String,
      jobTitle: json['job_title'] as String?,
      jobSlug: json['job_slug'] as String?,
      status: json['status'] as String,
      employerNotes: json['employer_notes'] as String?,
      appliedAt: _parseDateTime(json['applied_at']),
      statusUpdatedAt: _parseDateTime(json['status_updated_at']),
    );
  }

  static DateTime? _parseDateTime(dynamic value) {
    if (value == null) return null;
    if (value is DateTime) return value;
    if (value is String) return DateTime.tryParse(value);
    return null;
  }

  Map<String, dynamic> toJson() => {
        'id': id,
        'job_id': jobId,
        if (jobTitle != null) 'job_title': jobTitle,
        if (jobSlug != null) 'job_slug': jobSlug,
        'status': status,
        if (employerNotes != null) 'employer_notes': employerNotes,
        if (appliedAt != null) 'applied_at': appliedAt!.toIso8601String(),
        if (statusUpdatedAt != null)
          'status_updated_at': statusUpdatedAt!.toIso8601String(),
      };
}
