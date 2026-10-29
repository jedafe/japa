class FeatureFlags {
  final bool visaChecklist;
  final bool readiness;
  final bool costOfLiving;
  final bool scholarships;
  final bool remittance;
  final bool japabot;
  final Map<String, bool> flags;

  FeatureFlags({
    this.visaChecklist = false,
    this.readiness = false,
    this.costOfLiving = false,
    this.scholarships = false,
    this.remittance = false,
    this.japabot = false,
    this.flags = const {},
  });

  factory FeatureFlags.fromJson(Map<String, dynamic> json) {
    final allFlags = <String, bool>{};
    json.forEach((key, value) {
      if (value is bool) allFlags[key] = value;
    });
    return FeatureFlags(
      // Keys match the backend feature_flags table (internal/features/registry.go)
      visaChecklist: allFlags['checklist.enabled'] ?? false,
      readiness: allFlags['assessment.enabled'] ?? false,
      costOfLiving: allFlags['col_calculator.enabled'] ?? false,
      scholarships: allFlags['scholarship_finder.enabled'] ?? false,
      remittance: allFlags['remittance.enabled'] ?? false,
      japabot: allFlags['feature.japabot'] ?? false,
      flags: allFlags,
    );
  }

  bool isEnabled(String name) => flags[name] ?? false;

  Map<String, dynamic> toJson() => {
        ...flags,
        'visa_checklist': visaChecklist,
        'readiness': readiness,
        'cost_of_living': costOfLiving,
        'scholarships': scholarships,
        'remittance': remittance,
        'japabot': japabot,
      };
}
