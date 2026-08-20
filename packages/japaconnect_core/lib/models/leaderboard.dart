class Leaderboard {
  final String scope;
  final String period;
  final List<LeaderboardEntry> entries;

  Leaderboard({
    required this.scope,
    required this.period,
    required this.entries,
  });

  factory Leaderboard.fromJson(Map<String, dynamic> json) {
    return Leaderboard(
      scope: json['scope'] as String? ?? '',
      period: json['period'] as String? ?? '',
      entries: (json['entries'] as List?)
              ?.map((e) => LeaderboardEntry.fromJson(e as Map<String, dynamic>))
              .toList() ??
          [],
    );
  }

  Map<String, dynamic> toJson() => {
        'scope': scope,
        'period': period,
        'entries': entries.map((e) => e.toJson()).toList(),
      };
}

class LeaderboardEntry {
  final String userId;
  final String username;
  final String? displayName;
  final int points;

  LeaderboardEntry({
    required this.userId,
    required this.username,
    this.displayName,
    required this.points,
  });

  factory LeaderboardEntry.fromJson(Map<String, dynamic> json) {
    return LeaderboardEntry(
      userId: json['user_id'] as String? ?? json['id'] as String,
      username: json['username'] as String,
      displayName: json['display_name'] as String?,
      points: json['points'] as int? ?? 0,
    );
  }

  Map<String, dynamic> toJson() => {
        'user_id': userId,
        'username': username,
        if (displayName != null) 'display_name': displayName,
        'points': points,
      };
}
