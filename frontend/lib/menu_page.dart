import 'package:flutter/material.dart';

import 'api.dart';
import 'async_view.dart';
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
          for (final dish in entry.value) DishTile(dish: dish),
        ],
        const SizedBox(height: 16),
      ],
    );
  }
}

class DishTile extends StatelessWidget {
  final Dish dish;

  const DishTile({super.key, required this.dish});

  @override
  Widget build(BuildContext context) {
    return Card(
      margin: const EdgeInsets.symmetric(horizontal: 16, vertical: 4),
      child: ListTile(
        title: Text(dish.name),
        subtitle: dish.description.isEmpty ? null : Text(dish.description),
        trailing: Column(
          mainAxisSize: MainAxisSize.min,
          mainAxisAlignment: MainAxisAlignment.center,
          crossAxisAlignment: CrossAxisAlignment.end,
          children: [
            // Whole coins, so no decimal places. A 10-coin dish reads "10", not
            // "10.00", because fractional coins do not exist here.
            Text(
              '${dish.price} coins',
              style: Theme.of(context).textTheme.titleMedium,
            ),
            const SizedBox(height: 4),
            _AvailabilityChip(available: dish.available),
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
