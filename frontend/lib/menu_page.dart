import 'dart:async';

import 'package:flutter/material.dart';

import 'api.dart';
import 'models.dart';

class MenuPage extends StatefulWidget {
  const MenuPage({super.key});

  @override
  State<MenuPage> createState() => _MenuPageState();
}

class _MenuPageState extends State<MenuPage> {
  Future<List<Dish>>? _menuFuture;

  @override
  void initState() {
    super.initState();
    _menuFuture = Api.fetchMenu();
  }

  void _retry() {
    setState(() {
      _menuFuture = Api.fetchMenu();
    });
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Smart Canteen - Menu'),
      ),
      body: FutureBuilder<List<Dish>>(
        future: _menuFuture,
        builder: (context, snapshot) {
          if (snapshot.connectionState != ConnectionState.done) {
            return const Center(child: CircularProgressIndicator());
          }
          if (snapshot.hasError) {
            return Center(
              child: Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Text('Error: ${snapshot.error}'),
                  const SizedBox(height: 12),
                  FilledButton(
                    onPressed: _retry,
                    child: const Text('Retry'),
                  ),
                ],
              ),
            );
          }
          return _MenuList(dishes: snapshot.data!);
        },
      ),
    );
  }
}

class _MenuList extends StatelessWidget {
  final List<Dish> dishes;

  const _MenuList({required this.dishes});

  @override
  Widget build(BuildContext context) {
    if (dishes.isEmpty) {
      return const Center(child: Text('No dishes available'));
    }
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
              style: Theme.of(context).textTheme.titleMedium?.copyWith(
                    fontWeight: FontWeight.bold,
                  ),
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
            Text(
              dish.price.toStringAsFixed(2),
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