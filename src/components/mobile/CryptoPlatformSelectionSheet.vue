<template>
 <f7-popup :opened="opened" class="cy-mobile-surface" @popup:closed="emit('update:opened',false);emit('closed')">
  <f7-page class="cy-mobile-surface cy-asset-surface">
   <f7-navbar :title="kind==='WALLET'?'选择加密钱包':'选择加密货币交易所'"><f7-nav-left><f7-link icon-f7="xmark" aria-label="关闭选择" @click="emit('update:opened',false)" /></f7-nav-left></f7-navbar>
   <main class="platform-picker">
    <label class="platform-search"><f7-icon f7="search" /><input v-model="query" :placeholder="kind==='WALLET'?'搜索钱包':'搜索交易所'" :aria-label="kind==='WALLET'?'搜索钱包':'搜索交易所'" /></label>
    <section><button v-for="platform in filtered" :key="platform.id" type="button" @click="choose(platform)"><img v-if="platformIcon(platform.id)" :src="platformIcon(platform.id)" alt="" /><span v-else class="initial">{{platform.name.slice(0,1)}}</span><span>{{platform.name}}</span><f7-icon f7="chevron_right" /></button>
     <p v-if="!filtered.length" class="empty">没有找到匹配项，可以选择自定义。</p>
     <button type="button" @click="choose({id:kind==='WALLET'?'':'other-exchange',name:kind==='WALLET'?'自定义钱包':'自定义交易所',kind})"><span class="initial">＋</span><span>{{kind==='WALLET'?'自定义钱包':'自定义交易所'}}</span><f7-icon f7="chevron_right" /></button>
    </section>
   </main>
  </f7-page>
 </f7-popup>
</template>
<script setup lang="ts">
import {computed,ref,watch} from 'vue';
import {cryptoPlatforms,platformIcon,type CryptoPlatform} from '@/lib/crypto-platforms.ts';
const props=defineProps<{opened:boolean;kind:'EXCHANGE'|'WALLET'}>();
const emit=defineEmits<{'update:opened':[value:boolean];select:[platform:CryptoPlatform];closed:[]}>();
const query=ref('');
watch(()=>props.opened,open=>{if(open)query.value='';});
const filtered=computed(()=>cryptoPlatforms.filter(p=>p.kind===props.kind&&p.id!=='other-exchange'&&`${p.name} ${p.id} ${p.aliases||''}`.toLowerCase().includes(query.value.trim().toLowerCase())));
function choose(platform:CryptoPlatform):void {emit('select',platform);emit('update:opened',false);}
</script>
<style scoped>
.platform-picker{padding:14px;max-width:640px;margin:auto}.platform-search{display:flex;align-items:center;gap:10px;background:var(--cy-card);border-radius:10px;padding:12px 14px;margin-bottom:14px}.platform-search .icon{font-size:18px;color:var(--cy-muted)}.platform-search input{border:0;background:none;font-size:15px;color:var(--cy-ink);min-width:0;flex:1}.platform-picker section{background:var(--cy-card);border-radius:12px;padding:0 16px}.platform-picker button{display:flex;align-items:center;gap:15px;background:none;border:0;min-height:56px;width:100%;text-align:left;color:var(--cy-ink);font:inherit;font-size:15px}.platform-picker button>img,.initial{width:30px;height:30px;object-fit:contain;flex:none}.initial{display:grid;place-items:center;border-radius:9px;background:var(--cy-soft);color:var(--cy-accent);font-size:16px}.platform-picker button>.icon{margin-left:auto;color:var(--cy-muted);font-size:14px}.empty{font-size:13px;padding:16px 0;color:var(--cy-muted)}
</style>
