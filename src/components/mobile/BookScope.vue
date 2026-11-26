<template>
    <div v-if="flat" class="cy-book-scope cy-book-scope-flat">
        <div class="cy-book-scope-head"><span>账本范围</span><f7-link href="/books">管理账本</f7-link></div>
        <div class="cy-book-pills">
            <button type="button" :aria-pressed="!books.selectedBookIds.length" @click="books.selectedBookIds = []">全部账本</button>
            <button v-for="book in visibleBookList" :key="book.id" type="button" :aria-pressed="books.selectedBookIds.includes(book.id)" @click="toggle(book.id, !books.selectedBookIds.includes(book.id))">{{ book.icon }} {{ book.name }}<small v-if="book.archived">（归档）</small></button>
        </div>
        <p v-if="error" role="alert">{{ error }} <button @click="load">重试</button></p>
    </div>
    <details v-else class="cy-book-scope">
        <summary><span>{{ books.scopeName }}</span><span class="cy-muted">切换账本⌄</span></summary>
        <div class="cy-book-options">
            <label><input type="checkbox" :checked="!books.selectedBookIds.length" @change="books.selectedBookIds = []" />全部账本</label>
            <label v-for="book in visibleBookList" :key="book.id"><input type="checkbox" :checked="books.selectedBookIds.includes(book.id)" @change="toggle(book.id, ($event.target as HTMLInputElement).checked)" />{{ book.icon }} {{ book.name }}<small v-if="book.archived">已归档</small></label>
            <f7-link href="/books">管理账本</f7-link>
        </div>
        <p v-if="error" role="alert">{{ error }} <button @click="load">重试</button></p>
    </details>
</template>
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useBooksStore } from '@/stores/books.ts';
import { investmentError } from '@/lib/investments.ts';
withDefaults(defineProps<{ flat?: boolean }>(), { flat: false });
const books = useBooksStore(), error = ref('');
const visibleBookList = computed(() => books.allBooks.filter(item => !item.hidden || books.selectedBookIds.includes(item.id)));
function toggle(id: string, checked: boolean): void {
    books.selectedBookIds = checked ? [...books.selectedBookIds, id] : books.selectedBookIds.filter(value => value !== id);
}
async function load(): Promise<void> { error.value = ''; try { await books.loadBooks(); } catch(cause) { error.value = investmentError(cause); } }
onMounted(load);
</script>
<style scoped>
.cy-book-scope{border-bottom:1px solid var(--cy-line);margin-bottom:18px;padding-bottom:12px}.cy-book-scope summary{display:flex;justify-content:space-between;cursor:pointer;font-size:14px;list-style:none}.cy-book-scope summary span:last-child{font-size:12px}.cy-book-options{display:flex;flex-direction:column;gap:14px;padding:16px 2px 4px}.cy-book-options label{display:flex;gap:9px;align-items:center}.cy-book-options small{color:var(--cy-muted)}.cy-book-options input{accent-color:var(--cy-accent)}.cy-book-scope-head{display:flex;justify-content:space-between;align-items:center;font-size:13px;font-weight:600;margin-bottom:10px}.cy-book-scope-head a{font-size:12px;font-weight:400;color:var(--cy-accent)}.cy-book-pills{display:flex;flex-wrap:wrap;gap:8px}.cy-book-pills button{border:1px solid var(--cy-line);border-radius:16px;background:var(--cy-bg);color:var(--cy-ink);padding:6px 12px;font-size:12px}.cy-book-pills button[aria-pressed=true]{background:var(--cy-soft);border-color:var(--cy-accent);color:var(--cy-accent);font-weight:600}
</style>
