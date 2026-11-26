import type { LedgerBook } from '@/models/book.ts';
import type { TransactionCategory } from '@/models/transaction_category.ts';

// Visibility is presentation metadata. Existing category book restrictions still apply.
export function categoryAvailableInBook(category: Pick<TransactionCategory, 'id' | 'hidden' | 'bookIds'>, bookId: string, book?: LedgerBook): boolean {
    return !category.hidden && (!category.bookIds.length || category.bookIds.includes(bookId)) && !book?.hiddenCategoryIds?.includes(category.id);
}
