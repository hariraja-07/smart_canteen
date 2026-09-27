/// Prices and balances are whole coins: the database enforces it with a CHECK
/// constraint and the exchange rate is fixed, so these are ints rather than
/// doubles. A double here would let the UI print "10.00" for a 10-coin dish and
/// invite someone to add up money that is never fractional.
class Dish {
  final int id;
  final String name;
  final String category;
  final int price;
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
      price: (json['price'] as num).toInt(),
      description: json['description'] as String,
      available: json['available'] as bool,
    );
  }
}

/// The four roles the server recognises. A role is never inferred on the client;
/// it comes from the server, which is also what the server checks.
class Role {
  static const admin = 'admin';
  static const student = 'student';
  static const staff = 'staff';
  static const canteenManagement = 'canteen_management';

  /// The canteen and admin see the whole order queue; students and staff see
  /// only their own.
  static bool seesAllOrders(String role) =>
      role == admin || role == canteenManagement;
}

class User {
  final int id;
  final String name;
  final String email;
  final String role;
  final int coinBalance;

  const User({
    required this.id,
    required this.name,
    required this.email,
    required this.role,
    required this.coinBalance,
  });

  factory User.fromJson(Map<String, dynamic> json) {
    return User(
      id: json['id'] as int,
      name: json['name'] as String,
      email: json['email'] as String,
      role: json['role'] as String,
      coinBalance: (json['coin_balance'] as num).toInt(),
    );
  }

  User copyWith({int? coinBalance}) => User(
    id: id,
    name: name,
    email: email,
    role: role,
    coinBalance: coinBalance ?? this.coinBalance,
  );
}

/// Order lifecycle, mirroring the server's state machine. The client renders
/// these but the server decides which transition is legal.
class OrderStatus {
  static const pending = 'pending';
  static const preparing = 'preparing';
  static const ready = 'ready';
  static const completed = 'completed';
  static const cancelled = 'cancelled';

  /// The states a kitchen operator can still act on.
  static const actionable = [pending, preparing, ready];

  static String label(String status) {
    switch (status) {
      case pending:
        return 'Pending';
      case preparing:
        return 'Preparing';
      case ready:
        return 'Ready';
      case completed:
        return 'Completed';
      case cancelled:
        return 'Cancelled';
      default:
        return status;
    }
  }
}

class OrderItem {
  final int menuItemId;
  final String name;
  final int qty;
  final int unitPrice;
  final int lineTotal;

  const OrderItem({
    required this.menuItemId,
    required this.name,
    required this.qty,
    required this.unitPrice,
    required this.lineTotal,
  });

  factory OrderItem.fromJson(Map<String, dynamic> json) {
    return OrderItem(
      menuItemId: (json['menu_item_id'] as num).toInt(),
      name: json['name'] as String,
      qty: (json['qty'] as num).toInt(),
      unitPrice: (json['unit_price'] as num).toInt(),
      lineTotal: (json['line_total'] as num).toInt(),
    );
  }
}

class Order {
  final int id;
  final int userId;
  final String customer;
  final int total;
  final String status;
  final List<OrderItem> items;
  final String createdAt;
  final String updatedAt;

  const Order({
    required this.id,
    required this.userId,
    required this.customer,
    required this.total,
    required this.status,
    required this.items,
    required this.createdAt,
    required this.updatedAt,
  });

  factory Order.fromJson(Map<String, dynamic> json) {
    return Order(
      id: (json['id'] as num).toInt(),
      userId: (json['user_id'] as num).toInt(),
      customer: json['customer'] as String? ?? '',
      total: (json['total'] as num).toInt(),
      status: json['status'] as String,
      items: (json['items'] as List<dynamic>)
          .map((e) => OrderItem.fromJson(e as Map<String, dynamic>))
          .toList(),
      createdAt: json['created_at'] as String? ?? '',
      updatedAt: json['updated_at'] as String? ?? '',
    );
  }
}

/// One line of a coin history. A positive amount credits the user, a negative
/// one debits them.
class CoinEntry {
  final int id;
  final int amount;
  final String kind;
  final String reason;
  final int? actorId;
  final int? orderId;
  final String createdAt;

  const CoinEntry({
    required this.id,
    required this.amount,
    required this.kind,
    required this.reason,
    required this.actorId,
    required this.orderId,
    required this.createdAt,
  });

  factory CoinEntry.fromJson(Map<String, dynamic> json) {
    return CoinEntry(
      id: (json['id'] as num).toInt(),
      amount: (json['amount'] as num).toInt(),
      kind: json['kind'] as String,
      reason: json['reason'] as String? ?? '',
      actorId: (json['actor_id'] as num?)?.toInt(),
      orderId: (json['order_id'] as num?)?.toInt(),
      createdAt: json['created_at'] as String? ?? '',
    );
  }

  String get kindLabel {
    switch (kind) {
      case 'exchange_in':
        return 'Coins bought';
      case 'order_payment':
        return 'Order paid';
      case 'canteen_revenue':
        return 'Order revenue';
      case 'refund':
        return 'Refund';
      default:
        return kind;
    }
  }
}
