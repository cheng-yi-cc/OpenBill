import { computed, onScopeDispose, ref, watch } from 'vue';
import { LedgerDecimal } from '@/lib/ledger-display.ts';
import { projectRefundEntry } from '@/lib/refund-ledger.ts';
export { LedgerDecimal, ledgerMoney, ledgerCurrencyMoney, ledgerTotals, ledgerSignedAmount } from '@/lib/ledger-display.ts';
import moment from 'moment-timezone';
import services from '@/lib/services.ts';
import { investmentError } from '@/lib/investments.ts';
import { useAccountsStore } from '@/stores/account.ts';
import { useTransactionCategoriesStore } from '@/stores/transactionCategory.ts';
import { useTransactionTagsStore } from '@/stores/transactionTag.ts';
import { useBooksStore } from '@/stores/books.ts';
import { useLedgerScopeStore } from '@/stores/ledgerScope.ts';
import { useUserStore } from '@/stores/user.ts';
import type { TransactionInfoResponse } from '@/models/transaction.ts';
import type { TransactionPictureInfoBasicResponse } from '@/models/transaction_picture_info.ts';

export interface LedgerEntry {
    id: string; type: number; day: string; time: number; title: string; account: string;
    currency: string; amount: string; cny: string | null; hidden: boolean;
    categoryId: string; primaryCategoryId: string; primaryCategory: string; comment: string; tags: string[];
    bookId: string; accountId: string; destinationAccountId: string; tagIds: string[]; investment: boolean;
    investmentEventId?: string; excludeFromStatistics?: boolean; excludeFromBudget?: boolean; reimbursementAccountId?: string; reimbursementReceiptId?: string;
    wallet?: boolean;
    discountAmount?: string; transferFeeParentId?: string; accountFee?: string;
    originalAmount?: string; refundedAmount?: string; refundExpenseId?: string; refundId?: string; refundExcess?: boolean;
    bookName?: string; icon?: string; iconType?: number; color?: string; transferDirection?: 'in'|'out';
    accountIcon?: string; accountIconType?: number; pictures?: TransactionPictureInfoBasicResponse[];
    location?: string; reimbursementClosedAt?: number; destinationAccount?: string; sourceAccountName?:string; destinationCurrency?:string; destinationAmount?:string; transferFeeAmount?:string; debtDueDate?:string;
}
export function useMobileLedger(options: { applyFilters?: () => boolean; accountId?: () => string; bookIds?: () => string[] } = {}) {
    const accounts = useAccountsStore();
    const categories = useTransactionCategoriesStore();
    const tags = useTransactionTagsStore();
    const books = useBooksStore(), scope = useLedgerScopeStore();
    const allEntries = ref<LedgerEntry[]>([]);
    const entries = computed(() => options.applyFilters?.() === false ? allEntries.value : allEntries.value.filter(item =>
        (!scope.accountId || item.accountId === scope.accountId || item.destinationAccountId === scope.accountId) &&
        (!scope.categoryId || item.categoryId === scope.categoryId || item.primaryCategoryId === scope.categoryId) &&
        (!scope.tagId || item.tagIds.includes(scope.tagId)) && (!scope.type || item.type === scope.type)));
    const displayEntries = computed(() => entries.value.filter(item => {
        const book = books.allBooks.find(book => book.id === item.bookId);
        return item.investment ? book?.showInvestments !== false : item.type !== 4 || book?.showTransfers !== false;
    }));
    const loading = ref(true);
    const error = ref('');
    const hasMore = ref(false);
    let nextTime = 0;
    let requestNumber = 0;
    const users = useUserStore();
    watch(() => users.currentUserBasicInfo?.username, () => {
        requestNumber++;
        allEntries.value = [];
        error.value = '';
        loading.value = true;
    }, { flush: 'sync' });
    if (options.bookIds) {
        // Invalidate an in-flight result synchronously, including when the
        // caller switches away and back before either request returns.
        watch(() => options.bookIds!().join(','), () => {
            requestNumber++;
            allEntries.value = [];
            error.value = '';
            loading.value = true;
        }, { flush: 'sync' });
    }
    onScopeDispose(() => { requestNumber++; });

    function normalize(item: TransactionInfoResponse): LedgerEntry {
        const destination = accounts.allAccountsMap[item.destinationAccountId] || item.destinationAccount;
        const account = accounts.allAccountsMap[item.sourceAccountId] || item.sourceAccount || (item.investmentEventId ? destination : undefined);
        const category = categories.allTransactionCategoriesMap[item.categoryId] || item.category;
        const parent = category?.parentId ? categories.allTransactionCategoriesMap[category.parentId] : undefined;
        const incoming = !!options.accountId && item.type === 4 && item.destinationAccountId === options.accountId();
        const currency = item.wallet?.currency || (incoming ? destination : account)?.currency || '';
        const refundAccountView = !!options.accountId && item.refund?.kind === 'cash';
        const sourceAmount = new LedgerDecimal((incoming ? item.destinationAmount : item.sourceAmount).toString()).div(100);
        const amount = item.wallet?.amount || (refundAccountView ? sourceAmount.abs() : sourceAmount).toString();
        // Legacy daily transactions have no confirmed historical FX. Never substitute today's FX.
        const converted = item.wallet ? new LedgerDecimal(item.sourceAmount).div(100).toFixed() : currency === 'CNY' ? amount : null;
        return {
            id: item.id, type: refundAccountView ? 2 : item.type, excludeFromStatistics: !!item.excludeFromStatistics || (!!item.reimbursementAccountId && item.reimbursementAccountId !== '0'), reimbursementAccountId: item.reimbursementAccountId, reimbursementReceiptId: item.reimbursementReceiptId, day: moment.unix(item.time).tz(scope.timeZone).format('YYYY-MM-DD'), time: item.time,
            excludeFromBudget: item.excludeFromBudget ?? !!item.excludeFromStatistics,
            discountAmount: item.discountAmount || '0', transferFeeParentId:item.transferFeeParentId,
            title: refundAccountView ? '退款' : item.investmentEventId && !item.wallet ? (item.investmentSettlement ? `${item.investmentSettlement.type === 'BUY' ? '买入' : '卖出'}-${item.investmentSettlement.name}` : ['投资买入','投资卖出'].includes(item.comment) ? item.comment : '投资结算') : category?.name || ({ 1: '余额调整', 2: '收入', 3: '支出', 4: '账户转账' }[item.type] || '账户转账'),
            primaryCategory: parent?.name || category?.name || '未分类', primaryCategoryId: parent?.id || item.categoryId, categoryId: item.categoryId,
            bookId: item.bookId || '', accountId: item.wallet?.accountId || item.sourceAccountId, destinationAccountId: item.destinationAccountId,
            tagIds: item.tagIds || [], investment: !!item.investmentEventId && !item.wallet, investmentEventId: item.investmentEventId, wallet:!!item.wallet,
            account: item.wallet?.accountName || `${account?.assetProfile?.shortName || account?.name || '账户'}${item.type === 4 && destination && !item.investmentEventId ? ` → ${destination.assetProfile?.shortName||destination.name}` : ''}`,
            currency, amount, cny: converted, hidden: item.hideAmount, comment: item.comment,
            tags: (item.tagIds || []).map(id => tags.allTransactionTagsMap[id]?.name).filter((name): name is string => !!name),
            bookName: books.allBooks.find(b=>b.id===item.bookId)?.name || '日常账本', icon: category?.icon, iconType: category?.iconType, color: category?.color,
            accountIcon: account?.icon, accountIconType: account?.iconType, pictures: item.pictures,
            location: item.locationName || (item.geoLocation ? `${item.geoLocation.latitude}, ${item.geoLocation.longitude}` : ''),
            refundExpenseId: item.refund?.expenseId, refundId: item.refund?.refundId,
            reimbursementClosedAt: item.reimbursementClosedAt, destinationAccount: destination?.name, sourceAccountName:account?.name, destinationCurrency:destination?.currency, destinationAmount:item.type===4?new LedgerDecimal(item.destinationAmount).div(100).toString():undefined, transferFeeAmount:item.transferFeeAmount, debtDueDate:item.debtDueDate,
            transferDirection: options.accountId && item.type===4 ? incoming ? 'in' : 'out' : undefined
        };
    }
    let lastLoadedKey = '';
    async function load(startTime: number, endTime: number, opts?: { silent?: boolean }): Promise<void> {
        const version = ++requestNumber;
        const isCurrentScope = options.bookIds ? () => true : books.captureReportScope();
        const selectedBookIds = [...(options.bookIds?.() || books.selectedBookIds)];
        const scopeKey = `${Math.max(0, startTime)}:${endTime}:${selectedBookIds.join(',')}`;
        const preserveExisting = (opts?.silent ?? true) && allEntries.value.length > 0 && lastLoadedKey === scopeKey;
        if (!preserveExisting) {
            loading.value = true;
            allEntries.value = [];
        }
        error.value = '';
        try {
            const [, , , , response] = await Promise.all([
                accounts.loadAllAccounts({ force: true }).catch(keepUpToDate), categories.loadAllCategories({ force: false }).catch(keepUpToDate),
                tags.loadAllTags({ force: false }).catch(keepUpToDate), books.loadBooks(), services.getAllTransactions({ startTime: Math.max(0,startTime), endTime, withPictures: true, bookIds: [] })
            ]);
            if (version !== requestNumber || !isCurrentScope()) return;
            if (!response.data.success) throw new Error('账单加载失败，请重试。');
            const items = response.data.result;
            if (items.some(item => item.refund?.excess?.categoryId && !categories.allTransactionCategoriesMap[item.refund.excess.categoryId])) {
                await categories.loadAllCategories({ force: true }).catch(keepUpToDate);
            }
            if (version !== requestNumber || !isCurrentScope()) return;
            const tagNames = Object.fromEntries(Object.entries(tags.allTransactionTagsMap).map(([id, tag]) => [id, tag.name]));
            const bookNames = Object.fromEntries(books.allBooks.map(book => [book.id, book.name]));
            allEntries.value = items.flatMap(item => projectRefundEntry(normalize(item), item.refund, categories.allTransactionCategoriesMap, tagNames, bookNames))
                .filter(item => !selectedBookIds.length || selectedBookIds.includes(item.bookId))
                .sort((a, b) => b.time - a.time || b.id.localeCompare(a.id));
            lastLoadedKey = scopeKey;
        } catch (cause) {
            if (version === requestNumber && isCurrentScope()) {
                if (!preserveExisting) allEntries.value = [];
                error.value = investmentError(cause, 'read');
            }
        } finally { if (version === requestNumber) loading.value = false; }
    }
    async function loadAccount(more=false):Promise<void> {
        if(more && (loading.value || !hasMore.value))return;
        const version=++requestNumber, accountId=options.accountId?.();
        if(!accountId)return;
        loading.value=true; error.value='';
        if(!more){allEntries.value=[];nextTime=0;hasMore.value=false;}
        try {
            if(!more)await Promise.all([accounts.loadAllAccounts({force:true}).catch(keepUpToDate),categories.loadAllCategories({force:false}).catch(keepUpToDate),tags.loadAllTags({force:false}).catch(keepUpToDate),books.loadBooks()]);
            const response=await services.getTransactions({bookIds:[],accountIds:accountId,maxTime:nextTime,minTime:0,count:50,page:1,withCount:false,type:0,categoryIds:'',tagFilter:'',amountFilter:'',keyword:'',matchMode:0});
            if(version!==requestNumber)return;
            if(!response.data.success)throw Error('账户流水加载失败，请重试。');
            const result=response.data.result, seen=new Set(allEntries.value.map(item=>item.id));
            allEntries.value.push(...result.items.filter(item=>!seen.has(item.id)).map(normalize));
            nextTime=result.nextTimeSequenceId||0;hasMore.value=nextTime>0;
        }catch(cause){if(version===requestNumber)error.value=investmentError(cause);}
        finally{if(version===requestNumber)loading.value=false;}
    }
    return { entries, displayEntries, loading, error, load, loadAccount, hasMore, normalize };
}
// Legacy stores report an unchanged force refresh by rejecting with this marker.
export function keepUpToDate(error: unknown): void {
    if (typeof error === 'object' && error !== null && 'isUpToDate' in error && error.isUpToDate === true) return;
    throw error;
}
