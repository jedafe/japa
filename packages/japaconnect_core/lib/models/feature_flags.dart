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
      visaChecklist: allFlags['visa_checklist'] ?? false,
      readiness: allFlags['readiness'] ?? false,
      costOfLiving: allFlags['cost_of_living'] ?? false,
      scholarships: allFlags['scholarships'] ?? false,
      remittance: allFlags['remittance'] ?? false,
      japabot: allFlags['japabot'] ?? false,
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
