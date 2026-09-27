import 'package:flutter/material.dart';

import 'api.dart';
import 'async_view.dart';
import 'models.dart';
import 'session.dart';

/// The user's coin history: every mint, payment, and refund, newest first.
///
/// This is the screen that makes the ledger legible. A balance on its own is
/// just a number, and a number that changes is hard to trust unless you can see
/// what moved it.
class CoinsPage extends StatelessWidget {
  const CoinsPage({super.key});

  @override
  Widget build(BuildContext context) {
    final session = SessionScope.of(context);
    final userId = session.user?.id;
    if (userId == null) return const Center(child: Text('Signed out'));

    return AsyncView<List<CoinEntry>>(
      // Rebuilt per load rather than captured, so signing in as someone else
      // cannot leave this reading the previous user's history.
      load: () => Api.fetchCoinHistory(userId),
      refreshable: true,
      isEmpty: (entries) => entries.isEmpty,
      emptyMessage: 'No coin activity yet',
      builder: (context, entries) => ListView.separated(
        padding: const EdgeInsets.all(12),
        itemCount: entries.length,
        separatorBuilder: (_, _) => const SizedBox(height: 8),
        itemBuilder: (context, i) => CoinEntryTile(entry: entries[i]),
      ),
    );
  }
}

class CoinEntryTile extends StatelessWidget {
  final CoinEntry entry;

  const CoinEntryTile({super.key, required this.entry});

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final spent = entry.amount < 0;
    return Card(
      margin: EdgeInsets.zero,
      child: ListTile(
        leading: Icon(
          spent ? Icons.arrow_upward : Icons.arrow_downward,
          color: spent ? Colors.red.shade700 : Colors.green.shade700,
        ),
        title: Text(entry.kindLabel),
        // The sign is the point of the row, so it is spelled out rather than
        // left to a colour that some users cannot see.
        subtitle: Text(
          '${entry.reason}  ${shortDate(entry.createdAt)}',
          maxLines: 1,
          overflow: TextOverflow.ellipsis,
        ),
        trailing: Text(
          spent ? '${entry.amount}' : '+${entry.amount}',
          style: theme.textTheme.titleMedium?.copyWith(
            color: spent ? Colors.red.shade700 : Colors.green.shade700,
            fontWeight: FontWeight.bold,
          ),
        ),
      ),
    );
  }
}
