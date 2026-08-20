class Scholarship {
  final String id;
  final String? guideId;
  final String? intelRecordId;
  final String? title;
  final String? slug;
  final String? summary;
  final String? description;
  final String? institution;
  final String? country;
  final String? levelOfStudy;
  final int? awardAmountUsd;
  final bool? isRenewable;
  final String? applicationDeadline;
  final String? applicationUrl;
  final String? eligibility;
  final String? howToApply;
  final String? contactEmail;
  final Map<String, String>? fieldValues;
  final bool isSaved;

  Scholarship({
    required this.id,
    this.guideId,
    this.intelRecordId,
    this.title,
    this.slug,
    this.summary,
    this.description,
    this.institution,
    this.country,
    this.levelOfStudy,
    this.awardAmountUsd,
    this.isRenewable,
    this.applicationDeadline,
    this.applicationUrl,
    this.eligibility,
    this.howToApply,
    this.contactEmail,
    this.fieldValues,
    this.isSaved = false,
  });

  factory Scholarship.fromJson(Map<String, dynamic> json) {
    final fv = json['field_values'];
    return Scholarship(
      id: json['id'] as String,
      guideId: json['guide_id'] as String?,
      intelRecordId: json['intel_record_id'] as String?,
      title: json['title'] as String?,
      slug: json['slug'] as String?,
      summary: json['summary'] as String?,
      description: json['description'] as String?,
      institution: json['institution'] as String?,
      country: json['country'] as String?,
      levelOfStudy: json['level_of_study'] as String?,
      awardAmountUsd: json['award_amount_usd'] as int?,
      isRenewable: json['is_renewable'] as bool?,
      applicationDeadline: json['application_deadline'] as String?,
      applicationUrl: json['application_url'] as String?,
      eligibility: json['eligibility'] as String?,
      howToApply: json['how_to_apply'] as String?,
      contactEmail: json['contact_email'] as String?,
      fieldValues: fv is Map<String, dynamic>
          ? fv.map((k, v) => MapEntry(k, v.toString()))
          : null,
      isSaved: json['is_saved'] as bool? ?? false,
    );
  }

  Map<String, dynamic> toJson() => {
        'id': id,
        if (guideId != null) 'guide_id': guideId,
        if (intelRecordId != null) 'intel_record_id': intelRecordId,
        if (title != null) 'title': title,
        if (slug != null) 'slug': slug,
        if (summary != null) 'summary': summary,
        if (description != null) 'description': description,
        if (institution != null) 'institution': institution,
        if (country != null) 'country': country,
        if (levelOfStudy != null) 'level_of_study': levelOfStudy,
        if (awardAmountUsd != null) 'award_amount_usd': awardAmountUsd,
        if (isRenewable != null) 'is_renewable': isRenewable,
        if (applicationDeadline != null) 'application_deadline': applicationDeadline,
        if (applicationUrl != null) 'application_url': applicationUrl,
        if (eligibility != null) 'eligibility': eligibility,
        if (howToApply != null) 'how_to_apply': howToApply,
        if (contactEmail != null) 'contact_email': contactEmail,
        if (fieldValues != null) 'field_values': fieldValues,
        'is_saved': isSaved,
      };
}
