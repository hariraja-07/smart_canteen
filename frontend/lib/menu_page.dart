import 'package:flutter/material.dart';

import 'api.dart';
import 'async_view.dart';
import 'cart.dart';
import 'models.dart';

/// The menu body, with no Scaffold of its own. The shell provides the app bar
/// and navigation, so embedding this in a tab must not produce two app bars.
class MenuPage extends StatelessWidget {
  const MenuPage({super.key});

  @override
  Widget build(BuildContext context) {
    return AsyncView<List<Dish>>(
      load: Api.fetchMenu,
      isEmpty: (dishes) => dishes.isEmpty,
      emptyMessage: 'No dishes available',
      builder: (context, dishes) => _MenuList(dishes: dishes),
    );
  }
}

class _MenuList extends StatelessWidget {
  final List<Dish> dishes;

  const _MenuList({required this.dishes});

  @override
  Widget build(BuildContext context) {
    // Grouped by category, in the order the categories first appear, so the
    // layout matches the order the canteen listed them in.
    final byCategory = <String, List<Dish>>{};
    for (final dish in dishes) {
      byCategory.putIfAbsent(dish.category, () => []).add(dish);
    }
    return ListView(
      children: [
        for (final entry in byCategory.entries) ...[
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 20, 16, 8),
            child: Text(
              entry.key,
              style: Theme.of(
                context,
              ).textTheme.titleMedium?.copyWith(fontWeight: FontWeight.bold),
            ),
          ),
          for (final dish in entry.value) _CartDishTile(dish: dish),
        ],
        const SizedBox(height: 16),
      ],
    );
  }
}

/// A dish row with its add-to-cart control.
///
/// A dish already in the cart swaps its single Add button for a stepper, so
/// adding a second one does not need a trip back to the same row.
class _CartDishTile extends StatelessWidget {
  final Dish dish;

  const _CartDishTile({required this.dish});

  @override
  Widget build(BuildContext context) {
    final cart = CartScope.of(context);
    final qty = cart.qtyOf(dish.id);
    final theme = Theme.of(context);

    return Card(
      margin: const EdgeInsets.symmetric(horizontal: 16, vertical: 4),
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
        child: Row(
          children: [
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(dish.name, style: theme.textTheme.titleMedium),
                  if (dish.description.isNotEmpty)
                    Text(dish.description, style: theme.textTheme.bodySmall),
                  const SizedBox(height: 4),
                  // Whole coins, so no decimal places. A 10-coin dish reads
                  // "10", not "10.00", because fractional coins do not exist.
                  Text(
                    '${dish.price} coins',
                    style: theme.textTheme.bodyMedium,
                  ),
                  const SizedBox(height: 4),
                  _AvailabilityChip(available: dish.available),
                ],
              ),
            ),
            if (!dish.available)
              // Nothing to tap: the server would reject the order anyway.
              const Padding(
                padding: EdgeInsets.symmetric(horizontal: 8),
                child: Text('unavailable'),
              )
            else if (qty == 0)
              FilledButton.tonal(
                onPressed: () => cart.add(dish),
                child: const Text('Add'),
              )
            else
              Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  IconButton(
                    tooltip: 'Remove one',
                    icon: const Icon(Icons.remove_circle_outline),
                    onPressed: () => cart.removeOne(dish.id),
                  ),
                  Text('$qty', style: theme.textTheme.titleMedium),
                  IconButton(
                    tooltip: 'Add one',
                    icon: const Icon(Icons.add_circle_outline),
                    onPressed: () => cart.add(dish),
                  ),
                ],
              ),
          ],
        ),
      ),
    );
  }
}

class _AvailabilityChip extends StatelessWidget {
  final bool available;

  const _AvailabilityChip({required this.available});

  @override
  Widget build(BuildContext context) {
    final color = available ? Colors.green : Colors.grey;
    return Row(
      mainAxisSize: MainAxisSize.min,
      mainAxisAlignment: MainAxisAlignment.end,
      children: [
        Icon(
          available ? Icons.check_circle : Icons.cancel,
          size: 14,
          color: color,
        ),
        const SizedBox(width: 4),
        Text(
          available ? 'Available' : 'Sold Out',
          style: TextStyle(fontSize: 12, color: color),
        ),
      ],
    );
  }
}
