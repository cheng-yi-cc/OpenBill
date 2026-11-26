<template>
    <f7-page class="cy-main-page cy-mobile-surface" @page:afterin="load">
        <f7-navbar title="账本管理" back-link="返回">
            <f7-nav-right>
                <f7-link v-if="books.visibleBooks.length >= 2 && !showHidden" :class="{ disabled: busy }" @click="orderError = ''; orderOpen = true">排序</f7-link>
                <f7-link @click="openCreate">新增</f7-link>
            </f7-nav-right>
        </f7-navbar>
        <main ref="booksMainEl" class="cy-page-body cy-books-body" @click="menuId = ''">
            <p class="cy-books-tip">用账本区分日常、旅行等记录。各账本共用资金账户，切换账本不改变余额。</p>
            <p v-if="error || ledger.error.value" class="cy-message" role="alert">{{ error || ledger.error.value }}</p>
            <p v-if="message" role="status">{{ message }}</p>
            <button type="button" class="cy-books-hidden-toggle" @click="toggleHiddenView">{{ showHidden ? '返回账本列表' : `已隐藏账本（${hiddenBooks.length}）` }}</button>
            <p v-if="showHidden && !hiddenBooks.length" class="cy-book-hint">暂无隐藏的账本。</p>
            <section v-for="book in showHidden ? hiddenBooks : books.visibleBooks" :key="book.id" class="cy-panel cy-book-card" :class="{'cy-book-default': books.selectedBookIds.length === 1 && books.selectedBookIds[0] === book.id}" @click="selectBook(book)">
                <div class="cy-book-heading">
                    <span class="cy-book-icon" aria-hidden="true">{{ book.icon || '📒' }}</span>
                    <div class="cy-book-title"><button type="button" :disabled="book.archived || book.hidden" @click.stop="selectBook(book)">{{ book.name }}</button><small v-if="book.isDefault">默认账本</small><small v-if="book.archived">已归档</small></div>
                    <button v-if="showHidden" type="button" class="cy-books-show" :disabled="busy" @click.stop="confirmHidden(book)">显示</button>
                    <button v-else type="button" class="cy-book-menu-button" :aria-label="'管理' + book.name" :aria-expanded="menuId === book.id" @click.stop="menuId = menuId === book.id ? '' : book.id"><f7-icon f7="ellipsis" /></button>
                </div>
                <div class="cy-book-summary" :aria-label="book.name + '收支汇总'">
                    <div><span>总支出</span><strong class="cy-book-expense">{{ amount(book.id, 'expense') }}</strong></div>
                    <div><span>总收入</span><strong>{{ amount(book.id, 'income') }}</strong></div>
                    <div><span>结余</span><strong>{{ amount(book.id, 'balance') }}</strong></div>
                </div>
                <div class="cy-book-footer"><span>{{ ledger.loading.value ? '正在加载账单' : ledger.error.value ? '账单加载失败' : '共 ' + (summaries[book.id]?.count ?? 0) + ' 条账单' }}</span><span>账户余额共用</span></div>
                <Transition name="cy-pop"><div v-if="menuId === book.id" class="cy-book-menu" @click.stop>
                    <button type="button" @click="openEdit(book)"><f7-icon f7="pencil" />编辑账本</button>
                    <button v-if="!book.archived" type="button" :disabled="busy || (summaries[book.id]?.count ?? 0) === 0 || books.activeBooks.filter(b => !b.hidden && b.id !== book.id).length === 0" @click="openMove(book)"><f7-icon f7="arrow_right_square" />账单迁移</button>
                    <button v-if="!book.isDefault && books.visibleBooks.length > 1" type="button" :disabled="busy" @click="confirmHidden(book)"><f7-icon f7="eye_slash" />隐藏账本</button>
                    <button v-if="!book.isDefault && !book.archived" type="button" :disabled="busy" @click="setDefault(book)"><f7-icon f7="checkmark_circle" />设为默认</button>
                    <button v-if="!book.isDefault" type="button" :disabled="busy" @click="setArchived(book)"><f7-icon :f7="book.archived ? 'arrow_uturn_left' : 'archivebox'" />{{ book.archived ? '恢复账本' : '归档账本' }}</button>
                </div></Transition>
            </section>
        </main>
        <ManagementOrderPopup v-model:open="orderOpen" title="账本排序" :items="books.visibleBooks" :busy="busy" :error="orderError" @save="saveBookOrder" />
        <f7-popup v-model:opened="moveOpen" class="cy-mobile-surface">
            <f7-page>
                <f7-navbar title="账单迁移"><f7-nav-left><f7-link @click="moveOpen = false">取消</f7-link></f7-nav-left></f7-navbar>
                <main class="cy-page-body cy-book-editor">
                    <p v-if="moveError" class="cy-message" role="alert">{{ moveError }}</p>
                    <section class="cy-panel">
                        <h2 class="cy-book-move-title">账单迁移</h2>
                        <p>将“{{ moveSource?.name }}”的全部账单迁移到其他账本。</p>
                        <label class="cy-book-row"><span>目标账本</span><select v-model="moveTargetId" aria-label="迁移目标账本"><option value="" disabled>请选择</option><option v-for="book in moveTargets" :key="book.id" :value="book.id">{{ book.name }}</option></select></label>
                        <p class="cy-book-hint">退款、报销到账、转账关联流水及投资定投记录会一起迁移，账户余额与持仓成本不变。</p>
                        <button type="button" class="cy-book-move-submit" :disabled="busy || !moveTargetId" @click="confirmMove">迁移账单</button>
                    </section>
                </main>
            </f7-page>
        </f7-popup>
        <f7-popup v-model:opened="editorOpen" class="cy-mobile-surface" @popup:closed="closeEditor">
            <f7-page>
                <f7-navbar :title="editing ? '编辑账本' : '新增账本'">
                    <f7-nav-left><f7-link @click="editorOpen = false">取消</f7-link></f7-nav-left>
                    <f7-nav-right><f7-link :class="{disabled: busy}" @click="save">保存</f7-link></f7-nav-right>
                </f7-navbar>
                <main class="cy-page-body cy-book-editor">
                    <p v-if="editError" class="cy-message" role="alert">{{ editError }}</p>
                    <section class="cy-panel">
                        <label class="cy-book-name"><span class="cy-sr-only">账本名称</span><input v-model="draftName" maxlength="64" placeholder="账本名称" /></label>
                        <div class="cy-book-icon-picker"><span>账本图标</span><div class="cy-book-icon-grid"><button v-for="icon in icons" :key="icon" type="button" :aria-pressed="draftIcon === icon" @click="draftIcon = icon">{{ icon }}</button></div></div>
                    </section>
                    <section class="cy-panel cy-book-options">
                        <label v-if="!editing" class="cy-book-option"><span><strong>隐藏现有分类</strong><small>新账本先隐藏现有收支分类，可在分类管理中调整</small></span><input v-model="draftHideCategories" type="checkbox" /></label>
                        <label class="cy-book-option"><span><strong>显示转账流水</strong><small>在首页和日历中显示转账流水</small></span><input v-model="draftShowTransfers" type="checkbox" /></label>
                        <label class="cy-book-option"><span><strong>显示理财流水</strong><small>在首页和日历中显示投资结算流水</small></span><input v-model="draftShowInvestments" type="checkbox" /></label>
                    </section>
                    <p class="cy-book-hint">流水显示设置只影响列表，不改变收支和账户余额。</p>
                </main>
            </f7-page>
        </f7-popup>
    </f7-page>
</template>
<script setup lang="ts">
import { computed, ref } from 'vue';
import moment from 'moment-timezone';
import ManagementOrderPopup from '@/components/mobile/ManagementOrderPopup.vue';
import { useBooksStore } from '@/stores/books.ts';
import type { LedgerBook } from '@/models/book.ts';
import { investmentError } from '@/lib/investments.ts';
import { ledgerMoney, ledgerTotals, useMobileLedger } from '@/lib/mobile-ledger.ts';
import { statisticsEligible } from '@/lib/statistics-report.ts';
import { useI18nUIComponents } from '@/lib/ui/mobile.ts';
import { animateSettle } from '@/lib/motion.ts';
const booksMainEl = ref<HTMLElement>();
function toggleHiddenView(): void { showHidden.value = !showHidden.value; menuId.value = ''; animateSettle(booksMainEl.value, { y: 6, duration: 160 }); }

const books = useBooksStore();
const { showConfirm, showToast } = useI18nUIComponents();
const ledger = useMobileLedger({ applyFilters: () => false, bookIds: () => [] });
const error = ref(''), message = ref(''), editError = ref(''), busy = ref(false), editorOpen = ref(false), menuId = ref('');
const orderOpen = ref(false), orderError = ref('');
const showHidden = ref(false);
const moveOpen = ref(false), moveError = ref(''), moveTargetId = ref('');
const moveSource = ref<LedgerBook | null>(null);
const moveTargets = computed(() => books.activeBooks.filter(book => !book.hidden && book.id !== moveSource.value?.id));
const editing = ref<LedgerBook | null>(null), draftName = ref(''), draftIcon = ref('📒');
const draftShowTransfers = ref(true), draftShowInvestments = ref(true);
const draftHideCategories = ref(false);
const icons = ['📒', '🏠', '✈️', '🍃', '🎓', '💼', '🎁', '🚗'];
const hiddenBooks = computed(() => books.allBooks.filter(book => book.hidden));
const summaries = computed(() => Object.fromEntries(books.allBooks.map(book => {
    const entries = ledger.entries.value.filter(entry => entry.bookId === book.id);
    return [book.id, { ...ledgerTotals(entries.filter(statisticsEligible)), count: entries.filter(entry => entry.type !== 1).length }];
})));
function amount(id: string, field: 'income' | 'expense' | 'balance'): string {
    const item = summaries.value[id];
    return ledger.loading.value || ledger.error.value || !item?.complete ? '—' : ledgerMoney(item[field], false);
}
async function load(): Promise<void> {
    error.value = '';
    try { await books.loadBooks(true); await ledger.load(0, moment().add(100, 'year').unix()); }
    catch (cause) { error.value = investmentError(cause); }
}
function openCreate(): void { menuId.value = ''; editing.value = null; draftName.value = ''; draftIcon.value = '📒'; draftShowTransfers.value = true; draftShowInvestments.value = true; draftHideCategories.value = false; editError.value = ''; editorOpen.value = true; }
function openEdit(book: LedgerBook): void { menuId.value = ''; editing.value = { ...book }; draftName.value = book.name; draftIcon.value = book.icon || '📒'; draftShowTransfers.value = book.showTransfers; draftShowInvestments.value = book.showInvestments; editError.value = ''; editorOpen.value = true; }
function openMove(book: LedgerBook): void { menuId.value = ''; message.value = ''; moveSource.value = book; moveTargetId.value = ''; moveError.value = ''; moveOpen.value = true; }
function confirmMove(): void {
    const source = moveSource.value, target = moveTargets.value.find(book => book.id === moveTargetId.value);
    if (!source || !target || busy.value) return;
    showConfirm(`将“${source.name}”的全部账单迁移到“${target.name}”？关联流水会一并处理，账户余额不变。`, async () => {
        busy.value = true; moveError.value = '';
        try {
            const count = await books.moveAllTransactions(source.id, target.id);
            if (!count) { moveError.value = '此账本暂无可迁移的流水。'; return; }
            moveOpen.value = false; await load(); showToast(`已将 ${count} 条账单迁移到「${target.name}」`);
        }
        catch (cause) { moveError.value = investmentError(cause); }
        finally { busy.value = false; }
    }, undefined, { title: '账单迁移', confirmText: '确认迁移' });
}
function selectBook(book: LedgerBook): void { if (book.archived || book.hidden || busy.value) return; menuId.value = ''; books.selectedBookIds = [book.id]; showToast(`已切换到「${book.name}」`); }
function confirmHidden(book: LedgerBook): void {
    if (busy.value) return;
    if (!book.hidden && (book.isDefault || books.visibleBooks.length <= 1)) {
        showToast(book.isDefault ? '默认账本不能隐藏，请先将其他账本设为默认' : '至少保留一个可见账本');
        return;
    }
    menuId.value = '';
    const text = book.hidden ? '恢复后可在账本列表中选择。' : '隐藏后账本和账单仍保留，不会删除数据。';
    showConfirm(text, async () => {
        busy.value = true; error.value = '';
        try { await books.modifyBook({ ...book, hidden: !book.hidden }); showToast(book.hidden ? '账本已恢复显示' : '账本已隐藏'); }
        catch (cause) { error.value = investmentError(cause); }
        finally { busy.value = false; }
    }, undefined, { title: book.hidden ? '显示账本' : '隐藏账本', confirmText: book.hidden ? '显示' : '隐藏' });
}
function closeEditor(): void { if (!busy.value) { editing.value = null; editError.value = ''; } }
async function save(): Promise<void> {
    if (busy.value) return;
    const name = draftName.value.trim();
    if (!name) { editError.value = '请填写账本名称'; return; }
    busy.value = true; editError.value = '';
    try {
        if (editing.value) {
            await books.modifyBook({ ...editing.value, name, icon: draftIcon.value, showTransfers: draftShowTransfers.value, showInvestments: draftShowInvestments.value });
            showToast('账本已保存');
        } else {
            await books.createBook(name, draftIcon.value, draftShowTransfers.value, draftShowInvestments.value, draftHideCategories.value);
            showToast('已新建账本');
        }
        editorOpen.value = false;
    } catch (cause) { editError.value = investmentError(cause); }
    finally { busy.value = false; }
}
async function saveBookOrder(ids: string[]): Promise<void> {
    if (busy.value) return;
    busy.value = true; orderError.value = '';
    try {
        for (let i = 0; i < ids.length; i++) {
            const book = books.allBooks.find(item => item.id === ids[i]);
            if (book && book.displayOrder !== i + 1) {
                await books.modifyBook({ ...book, displayOrder: i + 1 });
            }
        }
        orderOpen.value = false;
        showToast('账本顺序已保存');
    } catch (cause) { orderError.value = investmentError(cause); }
    finally { busy.value = false; }
}
async function setDefault(book: LedgerBook): Promise<void> {
    if (busy.value) return;
    busy.value = true; error.value = ''; menuId.value = '';
    try { await books.modifyBook({ ...book, isDefault: true }); showToast(`已将「${book.name}」设为默认账本`); } catch (cause) { error.value = investmentError(cause); } finally { busy.value = false; }
}
function setArchived(book: LedgerBook): void {
    if (busy.value) return;
    menuId.value = '';
    const archiving = !book.archived;
    showConfirm(archiving ? `归档「${book.name}」后将无法向其记新账，历史账单仍可查看，随时可以恢复。` : `恢复「${book.name}」后可继续记账。`, async () => {
        busy.value = true; error.value = '';
        try {
            await books.modifyBook({ ...book, archived: archiving });
            if (archiving && books.selectedBookIds.includes(book.id)) books.selectedBookIds = books.selectedBookIds.filter(id => id !== book.id);
            showToast(archiving ? '账本已归档' : '账本已恢复');
        } catch (cause) { error.value = investmentError(cause); } finally { busy.value = false; }
    }, undefined, { title: archiving ? '归档账本' : '恢复账本', confirmText: archiving ? '归档' : '恢复' });
}
</script>
<style scoped>
.cy-books-body{padding-top:12px}.cy-books-tip{padding:13px 15px;margin:0 0 16px;border-radius:12px;background:var(--cy-soft);color:var(--cy-accent);font-size:13px;line-height:1.5}
.cy-books-hidden-toggle{display:block;margin:0 0 14px auto;border:0;background:none;color:var(--cy-accent);font-size:13px}.cy-books-show{border:0;border-radius:18px;padding:8px 16px;background:var(--cy-soft);color:var(--cy-accent)}
.cy-book-card{position:relative;margin-bottom:14px;padding:16px!important;border:1px solid transparent}.cy-book-default{border-color:var(--cy-accent)}
.cy-book-heading{display:flex;align-items:center;gap:12px}.cy-book-icon{width:42px;height:42px;display:grid;place-items:center;border-radius:12px;background:var(--cy-soft);font-size:23px}.cy-book-title{flex:1;min-width:0;display:flex;align-items:center;gap:7px}.cy-book-title button{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;border:0;padding:4px 0;background:none;color:var(--cy-ink);font-size:17px;font-weight:600;text-align:left}.cy-book-title small{flex:none;padding:3px 6px;border-radius:5px;background:var(--cy-soft);color:var(--cy-muted);font-size:11px}.cy-book-menu-button{border:0;background:none;color:var(--cy-ink);padding:10px;font-size:21px}
.cy-book-summary{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));margin-top:15px;padding:13px;border-radius:12px;background:var(--cy-soft)}.cy-book-summary>div{min-width:0}.cy-book-summary>div:last-child{text-align:right}.cy-book-summary span{display:block;color:var(--cy-muted);font-size:12px}.cy-book-summary strong{display:block;overflow:hidden;text-overflow:ellipsis;margin-top:5px;color:var(--cy-accent);font-size:16px}.cy-book-summary strong.cy-book-expense{color:#e85b59}.cy-book-footer{display:flex;justify-content:space-between;gap:8px;margin-top:14px;color:var(--cy-muted);font-size:12px}
.cy-book-menu{position:absolute;right:18px;top:56px;z-index:5;min-width:150px;padding:6px;background:var(--cy-card);border:1px solid var(--cy-line);border-radius:12px;box-shadow:0 5px 18px #0002;transform-origin:top right}.cy-book-menu button{display:flex;align-items:center;gap:10px;width:100%;padding:12px;border:0;background:none;color:var(--cy-ink);text-align:left}.cy-book-menu .icon{font-size:19px}
.cy-book-editor{padding-top:14px}.cy-book-name{display:block;padding:9px 0 16px}.cy-book-name input{width:100%;border:0;outline:0;background:transparent;color:var(--cy-ink);font-size:19px}.cy-book-icon-picker{padding-top:10px;border-top:1px solid var(--cy-line)}.cy-book-icon-picker>span{display:block;font-size:13px;color:var(--cy-muted);margin-bottom:10px}.cy-book-icon-grid{display:grid;grid-template-columns:repeat(8,minmax(0,1fr));gap:6px}.cy-book-icon-grid button{aspect-ratio:1;border:1px solid transparent;border-radius:10px;background:var(--cy-bg);font-size:20px;display:grid;place-items:center;padding:0}.cy-book-icon-grid button[aria-pressed=true]{background:var(--cy-soft);border-color:var(--cy-accent)}.cy-sr-only{position:absolute;width:1px;height:1px;overflow:hidden;clip:rect(0,0,0,0)}.cy-book-row,.cy-book-option{display:flex;align-items:center;justify-content:space-between;gap:16px;padding:15px 0}.cy-book-row select,.cy-book-option input[type=number]{border:0;background:transparent;color:var(--cy-ink);font-size:18px}.cy-book-option+.cy-book-option{border-top:1px solid var(--cy-line)}.cy-book-option span{display:flex;flex-direction:column;gap:5px}.cy-book-option strong{font-weight:500;font-size:15px}.cy-book-option small,.cy-book-hint{color:var(--cy-muted);font-size:12px}.cy-book-option input[type=checkbox]{width:21px;height:21px;accent-color:var(--cy-accent)}.cy-book-option input[type=number]{width:70px;text-align:right}
.cy-book-move-submit{width:100%;padding:12px;border:0;border-radius:10px;background:var(--cy-accent);color:white;font-size:15px}.cy-book-move-submit:disabled{opacity:.5}
.cy-book-move-title{margin:0;font-size:18px;font-weight:600}
</style>
