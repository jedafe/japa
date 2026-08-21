class FeatureFlags {
  final bool visaChecklist;
  final bool readiness;
  final bool costOfLiving;
  final bool scholarships;
  final bool remittance;
  final bool japabot;

  FeatureFlags({
    this.visaChecklist = false,
    this.readiness = false,
    this.costOfLiving = false,
    this.scholarships = false,
    this.remittance = false,
    this.japabot = false,
  });

  factory FeatureFlags.fromJson(Map<String, dynamic> json) {
    return FeatureFlags(
      visaChecklist: json['visa_checklist'] == true,
      readiness: json['readiness'] == true,
      costOfLiving: json['cost_of_living'] == true,
      scholarships: json['scholarships'] == true,
      remittance: json['remittance'] == true,
      japabot: json['japabot'] == true,
    );
  }

  Map<String, dynamic> toJson() => {
        'visa_checklist': visaChecklist,
        'readiness': readiness,
        'cost_of_living': costOfLiving,
        'scholarships': scholarships,
        'remittance': remittance,
        'japabot': japabot,
      };
}
