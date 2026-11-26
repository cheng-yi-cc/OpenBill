export interface EntryAccountChoice { id: string; currency: string }

export function resolveRouteEntryAccount(routeAccountId: string, accounts: EntryAccountChoice[]): string {
    if (!accounts.some(account => account.id === routeAccountId)) throw new Error('指定账户不可用，请返回后重新选择账户');
    return routeAccountId;
}

// A route-selected or manually selected account is an explicit instruction.
// Category defaults are suggestions and may only replace an implicit selection.
export function resolveEntryAccount(current: string, explicitlySelected: boolean, preference: string,
                                    categoryId: string, categoryAccounts: Record<string, string>, lastAccountId: string,
                                    accounts: EntryAccountChoice[], multiCurrency: boolean): string {
    if (explicitlySelected) return current;
    const suggested = preference === 'category' ? categoryAccounts[categoryId] : preference === 'last' ? lastAccountId : preference;
    return suggested && accounts.some(account => account.id === suggested && (multiCurrency || account.currency === 'CNY'))
        ? suggested : current;
}
