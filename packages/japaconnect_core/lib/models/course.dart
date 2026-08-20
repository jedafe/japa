class Course {
  final String id;
  final String title;
  final String slug;
  final String? description;
  final String? difficulty;
  final String? level;
  final String? currency;
  final String? language;
  final int? totalEnrolled;
  final bool? subscriptionEligible;
  final double? avgRating;
  final int? reviewCount;

  Course({
    required this.id,
    required this.title,
    required this.slug,
    this.description,
    this.difficulty,
    this.level,
    this.currency,
    this.language,
    this.totalEnrolled,
    this.subscriptionEligible,
    this.avgRating,
    this.reviewCount,
  });

  factory Course.fromJson(Map<String, dynamic> json) {
    return Course(
      id: json['id'] as String,
      title: json['title'] as String,
      slug: json['slug'] as String,
      description: json['description'] as String?,
      difficulty: json['difficulty'] as String?,
      level: json['level'] as String?,
      currency: json['currency'] as String?,
      language: json['language'] as String?,
      totalEnrolled: json['total_enrolled'] as int?,
      subscriptionEligible: json['subscription_eligible'] as bool?,
      avgRating: (json['avg_rating'] as num?)?.toDouble(),
      reviewCount: json['review_count'] as int?,
    );
  }

  Map<String, dynamic> toJson() => {
        'id': id,
        'title': title,
        'slug': slug,
        if (description != null) 'description': description,
        if (difficulty != null) 'difficulty': difficulty,
        if (level != null) 'level': level,
        if (currency != null) 'currency': currency,
        if (language != null) 'language': language,
        if (totalEnrolled != null) 'total_enrolled': totalEnrolled,
        if (subscriptionEligible != null) 'subscription_eligible': subscriptionEligible,
        if (avgRating != null) 'avg_rating': avgRating,
        if (reviewCount != null) 'review_count': reviewCount,
      };
}

class Lesson {
  final String id;
  final String title;
  final String? body;

  Lesson({
    required this.id,
    required this.title,
    this.body,
  });

  factory Lesson.fromJson(Map<String, dynamic> json) {
    return Lesson(
      id: json['id'] as String,
      title: json['title'] as String,
      body: json['body'] as String?,
    );
  }

  Map<String, dynamic> toJson() => {
        'id': id,
        'title': title,
        if (body != null) 'body': body,
      };
}
