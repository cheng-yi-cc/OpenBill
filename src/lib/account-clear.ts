import { useUserStore } from '@/stores/user.ts';
import { useRootStore } from '@/stores/index.ts';
import { useAccountsStore } from '@/stores/account.ts';

export async function isPasswordlessUser(): Promise<boolean> {
    const profile = await useUserStore().getCurrentUserProfile();
    return !!profile.noPassword;
}

export async function clearSingleAccountTransactions(accountId: string, password: string): Promise<void> {
    await useRootStore().clearAllUserTransactionsOfAccount({ accountId, password });
    await useAccountsStore().loadAllAccounts({ force: true });
}
