class Dish {
  final int id;
  final String name;
  final String category;
  final double price;
  final String description;
  final bool available;

  const Dish({
    required this.id,
    required this.name,
    required this.category,
    required this.price,
    required this.description,
    required this.available,
  });

  factory Dish.fromJson(Map<String, dynamic> json) {
    return Dish(
      id: json['id'] as int,
      name: json['name'] as String,
      category: json['category'] as String,
      price: (json['price'] as num).toDouble(),
      description: json['description'] as String,
      available: json['available'] as bool,
    );
  }
}