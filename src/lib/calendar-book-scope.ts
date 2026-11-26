import type { LedgerBook } from '@/models/book.ts';

// An empty persisted scope means all books. Expand it in the picker so that
// choosing all visible books cannot silently include hidden ones.
export function calendarBookDraft(current: string[], books: LedgerBook[]): string[] {
    return current.length ? [...current] : books.filter(book => !book.hidden).map(book => book.id);
}

// Once the user changes a visible choice, an old hidden choice must not keep
// contributing silently to the newly selected reporting scope.
export function toggleVisibleCalendarBook(draft: string[], books: LedgerBook[], id: string, checked: boolean): string[] {
    const visible = books.filter(book => !book.hidden).map(book => book.id);
    const ids = draft.filter(item => visible.includes(item) && item !== id);
    if (checked && visible.includes(id)) ids.push(id);
    return ids;
}

export function calendarBookScope(draft: string[], books: LedgerBook[]): string[] | null {
    const ids = [...new Set(draft)];
    if (!ids.length || ids.some(id => !books.some(book => book.id === id))) return null;
    if (books.length === ids.length && books.every(book => !book.hidden && ids.includes(book.id))) return [];
    // The server validates an explicit filter at 100 identities.
    return ids.length <= 100 ? ids : null;
}
