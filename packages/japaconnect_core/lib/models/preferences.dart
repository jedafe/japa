class UserPreferences {
  final String? theme;
  final String? language;
  final bool? emailNotifications;
  final bool? pushNotifications;

  UserPreferences({
    this.theme,
    this.language,
    this.emailNotifications,
    this.pushNotifications,
  });

  factory UserPreferences.fromJson(Map<String, dynamic> json) {
    return UserPreferences(
      theme: json['theme'] as String?,
      language: json['language'] as String?,
      emailNotifications: json['email_notifications'] as bool?,
      pushNotifications: json['push_notifications'] as bool?,
    );
  }

  Map<String, dynamic> toJson() => {
        if (theme != null) 'theme': theme,
        if (language != null) 'language': language,
        if (emailNotifications != null) 'email_notifications': emailNotifications,
        if (pushNotifications != null) 'push_notifications': pushNotifications,
      };
}
