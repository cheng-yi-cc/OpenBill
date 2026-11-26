<template>
    <f7-link :id="menuId" icon-f7="ellipsis_vertical" aria-label="账户更多操作" @click="showMenu=true" />
    <f7-popover :target-el="`#${menuId}`" v-model:opened="showMenu">
        <f7-list>
            <f7-list-item v-for="item in items" :key="item.title" :title="item.title" :link="item.href||'#'" popover-close @click="item.action&&emit('action',item.action)"/>
            <f7-list-item :title="editTitle||'编辑账户'" :link="editHref" popover-close />
        </f7-list>
        <f7-list class="cy-danger-menu-group">
            <f7-list-item v-if="target.kind==='cash'" title="清除流水" link="#" class="text-color-red" popover-close @click="startClear" />
            <f7-list-item :title="deleteTitle||'删除账户'" link="#" class="text-color-red" popover-close @click="showDeletion=true" />
        </f7-list>
    </f7-popover>
    <AccountDeletionSheet v-model:opened="showDeletion" :target="target" immediate @deleted="emit('deleted')" />
    <PasswordInputSheet v-model:show="showClearPassword" v-model="clearPassword" title="确认清除流水" :hint="`输入当前密码以清除「${target.name}」的全部流水`" color="red" :confirm-disabled="clearing" :cancel-disabled="clearing" @password:confirm="executeClear" />
</template>
<script setup lang="ts">
import {ref} from 'vue';
import {generateRandomUUID} from '@/lib/misc.ts';
import {clearSingleAccountTransactions,isPasswordlessUser} from '@/lib/account-clear.ts';
import {investmentError} from '@/lib/investments.ts';
import {useI18nUIComponents} from '@/lib/ui/mobile.ts';
import AccountDeletionSheet from '@/components/mobile/AccountDeletionSheet.vue';
import PasswordInputSheet from '@/components/mobile/PasswordInputSheet.vue';
const props=defineProps<{editHref:string;editTitle?:string;deleteTitle?:string;items?:{title:string;href?:string;action?:string}[];target:{id:string;name:string;kind:'cash'|'portfolio';href:string}}>();
const emit=defineEmits<{deleted:[];cleared:[];action:[value:string]}>();
const {showToast,showConfirm,showAlert}=useI18nUIComponents();
const menuId='account-menu-'+generateRandomUUID(),showMenu=ref(false),showDeletion=ref(false),showClearPassword=ref(false),clearPassword=ref(''),clearing=ref(false);
async function startClear():Promise<void>{
 if(clearing.value)return;
 try{
  if(await isPasswordlessUser()){
   showConfirm(`将清除「${props.target.name}」的全部流水并同步恢复对端账户余额，此操作无法撤销。`,()=>void executeClear(''),undefined,{title:'清除流水',confirmText:'清除',destructive:true,raw:true});
   return;
  }
  clearPassword.value='';
  showClearPassword.value=true;
 }catch(e){showAlert(investmentError(e),undefined,{title:'清除流水失败',raw:true});}
}
async function executeClear(password:string):Promise<void>{
 if(clearing.value)return;
 clearing.value=true;
 try{
  await clearSingleAccountTransactions(props.target.id,password);
  showClearPassword.value=false;
  clearPassword.value='';
  showToast('已清除账户流水');
  emit('cleared');
 }catch(e){showAlert(investmentError(e),undefined,{title:'清除流水失败',raw:true});}finally{clearing.value=false;}
}
</script>
<style scoped>
.cy-danger-menu-group{margin-top:0;border-top:6px solid var(--cy-bg)}
</style>
