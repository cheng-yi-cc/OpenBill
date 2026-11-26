import axios from 'axios';
import '@/lib/services.ts';
import type { ApiResponse } from '@/core/api.ts';
import type { RefundExcessInfo } from '@/models/transaction.ts';
import {useTransactionsStore} from '@/stores/transaction.ts';

export interface RefundRecord {
    id:string; expenseId:string; transactionId:string; accountId:string; amount:string;
    offsetAmount:string; excessAmount:string; time:number; timeZone:string; comment:string; installmentMode?:string; version:number;
    excess?:RefundExcessInfo;
}
export interface RefundSummary {
    expenseId:string; originalAmount:string; refundedAmount:string; netExpense:string; excessIncome:string;
    hasActiveInstallment?:boolean; supportsInstallmentReduce?:boolean; installmentPrincipal?:string; installmentUnaccruedPrincipal?:string;
    refunds:RefundRecord[];
}
export interface RefundInput {expenseId:string;accountId:string;amount:string;time:number;timeZone:string;comment:string;installmentMode?:string;requestId:string;expectedOriginalAmount:string;expectedRefundedAmount:string}
export interface RefundUpdateInput {id:string;accountId:string;amount:string;time:number;timeZone:string;comment:string;installmentMode?:string;version:number}
export interface RefundExcessInput extends RefundExcessInfo {id:string;version:number}
export async function loadRefunds(expenseId:string):Promise<RefundSummary> {
    const response=await axios.get<ApiResponse<RefundSummary>>('v1/refunds',{params:{expenseId}});
    if (!response.data.success) throw new Error('退款明细加载失败，请重试');
    return response.data.result;
}
async function writeRefund(path:string,data:unknown):Promise<RefundSummary> {
    const response=await axios.post<ApiResponse<RefundSummary>>(`v1/refunds/${path}`,data);
    if (!response.data.success) throw new Error('退款操作失败，请重试');
    useTransactionsStore().updateStoreInvalidState({accountList:true,transactionList:true,overview:true,statistics:true,explorer:true,reconciliationStatement:true});
    return response.data.result;
}
export const refundActions={
    create:(input:RefundInput)=>writeRefund('create',input),
    update:(input:RefundUpdateInput)=>writeRefund('update',input),
    updateExcess:(input:RefundExcessInput)=>writeRefund('excess',input),
    delete:(id:string,version:number)=>writeRefund('delete',{id,version})
};
