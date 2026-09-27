import 'package:flutter/widgets.dart';

import 'models.dart';

/// One line of the cart: the dish as it was added, and how many.
///
/// The dish is kept whole rather than just its id so the cart can show a name
/// and a price without refetching the menu, and so a line cannot change price
/// under the user after it was added.
class CartLine {
  final Dish dish;
  int qty;

  CartLine(this.dish, this.qty);

  int get lineTotal => dish.price * qty;
}

class Cart extends ChangeNotifier {
  final Map<int, CartLine> _lines = {};

  List<CartLine> get lines => _lines.values.toList(growable: false);
  bool get isEmpty => _lines.isEmpty;
  bool get isNotEmpty => _lines.isNotEmpty;
  int get total => _lines.values.fold(0, (sum, l) => sum + l.lineTotal);
  int get count => _lines.values.fold(0, (sum, l) => sum + l.qty);

  int qtyOf(int dishId) => _lines[dishId]?.qty ?? 0;

  /// Adds a dish. Refuses a sold-out dish, because the server would reject the
  /// order anyway and a cart that cannot be bought is a dead end.
  void add(Dish dish, [int by = 1]) {
    if (!dish.available || by <= 0) return;
    final line = _lines[dish.id];
    if (line == null) {
      _lines[dish.id] = CartLine(dish, by);
    } else {
      line.qty += by;
    }
    notifyListeners();
  }

  void removeOne(int dishId) {
    final line = _lines[dishId];
    if (line == null) return;
    if (line.qty <= 1) {
      _lines.remove(dishId);
    } else {
      line.qty--;
    }
    notifyListeners();
  }

  void setQty(int dishId, int qty) {
    if (qty <= 0) {
      if (_lines.remove(dishId) != null) notifyListeners();
      return;
    }
    final line = _lines[dishId];
    if (line == null) return;
    line.qty = qty;
    notifyListeners();
  }

  void clear() {
    if (_lines.isEmpty) return;
    _lines.clear();
    notifyListeners();
  }

  /// The request body: dish ids and quantities only. No prices, because the
  /// server prices the cart and a client that can name a price can set its own
  /// cost.
  Map<int, int> toRequest() => {
    for (final line in _lines.values) line.dish.id: line.qty,
  };
}

class CartScope extends InheritedNotifier<Cart> {
  const CartScope({super.key, required Cart cart, required super.child})
    : super(notifier: cart);

  static Cart of(BuildContext context) {
    final scope = context.dependOnInheritedWidgetOfExactType<CartScope>();
    assert(scope != null, 'No CartScope found in the widget tree');
    return scope!.notifier!;
  }

  static Cart read(BuildContext context) {
    final scope = context.getInheritedWidgetOfExactType<CartScope>();
    assert(scope != null, 'No CartScope found in the widget tree');
    return scope!.notifier!;
  }
}
