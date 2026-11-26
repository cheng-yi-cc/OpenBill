import { LedgerDecimal } from '@/lib/ledger-display.ts';
import type { LedgerEntry } from '@/lib/mobile-ledger.ts';
import type { RefundProjection } from '@/models/transaction.ts';

type Category = { id: string; name: string; parentId: string; icon?: string; iconType?: number; color?: string };

// Reporting projections never change the underlying account movement. The
// parent stays on its original day; any excess belongs to the refund day.
export function projectRefundEntry(entry: LedgerEntry, refund: RefundProjection | undefined,
    categories: Record<string, Category>, tagNames: Record<string, string>, bookNames: Record<string, string>): LedgerEntry[] {
    if (!refund) return [entry];
    const parse = (value: string | undefined): InstanceType<typeof LedgerDecimal> => {
        if (!value || !/^(0|[1-9]\d{0,12})(\.\d{1,2})?$/.test(value)) throw new Error('退款数据不完整，请核对账本');
        return new LedgerDecimal(value);
    };
    if (refund.kind === 'expense') {
        const net = parse(refund.netExpense), refunded = parse(refund.refundedAmount);
        if (entry.type !== 3 || net.gt(entry.amount)) throw new Error('退款原账金额不一致，请核对账本');
        return [{ ...entry, amount: net.toString(), cny: entry.cny === null ? null : net.toString(),
            originalAmount: entry.amount, refundedAmount: refunded.toString(), refundExpenseId: refund.expenseId }];
    }
    if (refund.kind !== 'cash') throw new Error('退款类型无效，请核对账本');
    const excess = parse(refund.excessAmount);
    if (excess.isZero()) return [];
    const info = refund.excess;
    const category = info && categories[info.categoryId];
    if (!info || !category || !info.bookId) throw new Error('退款差额信息不完整，请核对账本');
    const parent = categories[category.parentId];
    return [{ ...entry, type: 2, amount: excess.toString(), cny: entry.currency === 'CNY' ? excess.toString() : null,
        title: category.name, primaryCategory: parent?.name || category.name, primaryCategoryId: parent?.id || category.id,
        categoryId: category.id, icon: category.icon, iconType: category.iconType, color: category.color,
        account: '无账户', sourceAccountName: undefined, accountId: 'refund_unassigned', destinationAccountId: '', accountIcon: undefined, accountIconType: undefined,
        bookId: info.bookId, bookName: bookNames[info.bookId] || '账本', comment: info.comment,
        tagIds: info.tagIds, tags: info.tagIds.map(id => tagNames[id]).filter((name): name is string => !!name),
        excludeFromStatistics: info.excludeFromStatistics, excludeFromBudget: info.excludeFromBudget,
        refundExpenseId: refund.expenseId, refundId: refund.refundId, refundExcess: true }];
}
