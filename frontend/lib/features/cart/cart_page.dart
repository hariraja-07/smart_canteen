import 'package:flutter/material.dart';

import '../../core/api.dart';
import '../../core/async_action.dart';
import '../../core/cart.dart';
import '../../core/models.dart';
import '../../core/session.dart';

/// The cart, and the button that spends coins.
///
/// A successful order clears the cart and re-reads the balance, because both
/// are now wrong: the coins have moved, and leaving a placed order in the cart
/// invites paying for it twice.
class CartPage extends StatefulWidget {
  /// Called after a successful order so the shell can show the order list.
  final VoidCallback? onPlaced;

  const CartPage({super.key, this.onPlaced});

  @override
  State<CartPage> createState() => _CartPageState();
}

class _CartPageState extends State<CartPage> {
  bool _placing = false;

  Future<void> _place() async {
    final cart = CartScope.read(context);
    final session = SessionScope.read(context);
    if (cart.isEmpty || _placing) return;

    setState(() => _placing = true);
    try {
      final order = await runMutation(
        context,
        () => Api.placeOrder(cart.toRequest()),
      );
      if (order == null) return;
      // Cleared before the balance is refreshed. If the refresh were to fail,
      // the cart would still hold an order the server has already accepted,
      // and the next tap would place it a second time.
      cart.clear();
      // The balance in the app bar is the server's number, and this order
      // changed it.
      await session.refresh();
      if (!mounted) return;
      _confirm(order);
    } finally {
      if (mounted) setState(() => _placing = false);
    }
  }

  void _confirm(Order order) {
    showDialog<void>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Order placed'),
        content: Text(
          'Order #${order.id} for ${order.total} coins.\n'
          'The kitchen can see it now.',
        ),
        actions: [
          TextButton(
            onPressed: () {
              Navigator.of(context).pop();
              widget.onPlaced?.call();
            },
            child: const Text('View my orders'),
          ),
          FilledButton(
            onPressed: () => Navigator.of(context).pop(),
            child: const Text('Keep shopping'),
          ),
        ],
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final cart = CartScope.of(context);
    final session = SessionScope.of(context);
    final theme = Theme.of(context);

    if (cart.isEmpty) {
      return const Center(child: Text('Your cart is empty'));
    }

    // The pre-check is a courtesy. The server is still the authority, so a 402
    // is handled too: the balance can change between this build and the tap.
    final affordable = cart.total <= session.coinBalance;

    return Column(
      children: [
        Expanded(
          child: ListView.separated(
            padding: const EdgeInsets.all(12),
            itemCount: cart.lines.length,
            separatorBuilder: (_, _) => const Divider(height: 1),
            itemBuilder: (context, i) {
              final line = cart.lines[i];
              return ListTile(
                title: Text(line.dish.name),
                subtitle: Text('${line.dish.price} coins each'),
                trailing: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    IconButton(
                      tooltip: 'Remove one',
                      icon: const Icon(Icons.remove_circle_outline),
                      onPressed: () => cart.removeOne(line.dish.id),
                    ),
                    Text('${line.qty}', style: theme.textTheme.titleMedium),
                    IconButton(
                      tooltip: 'Add one',
                      icon: const Icon(Icons.add_circle_outline),
                      onPressed: () => cart.add(line.dish),
                    ),
                  ],
                ),
              );
            },
          ),
        ),
        const Divider(height: 1),
        Padding(
          padding: const EdgeInsets.fromLTRB(16, 12, 16, 16),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Row(
                children: [
                  Text('Total', style: theme.textTheme.titleMedium),
                  const Spacer(),
                  Text(
                    '${cart.total} coins',
                    style: theme.textTheme.titleMedium,
                  ),
                ],
              ),
              const SizedBox(height: 4),
              Text(
                'You have ${session.coinBalance} coins',
                style: theme.textTheme.bodySmall,
              ),
              if (!affordable) ...[
                const SizedBox(height: 8),
                Text(
                  'Not enough coins. Visit the canteen counter to top up.',
                  style: TextStyle(color: theme.colorScheme.error),
                ),
              ],
              const SizedBox(height: 12),
              FilledButton.icon(
                onPressed: (affordable && !_placing) ? _place : null,
                icon: _placing
                    ? const SizedBox(
                        height: 18,
                        width: 18,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      )
                    : const Icon(Icons.point_of_sale),
                label: Text(_placing ? 'Placing order' : 'Place order'),
              ),
              const SizedBox(height: 4),
              TextButton(
                onPressed: _placing ? null : cart.clear,
                child: const Text('Empty cart'),
              ),
            ],
          ),
        ),
      ],
    );
  }
}
