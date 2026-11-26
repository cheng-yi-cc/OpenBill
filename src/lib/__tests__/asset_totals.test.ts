import {describe,it,expect,vi} from 'vitest';
vi.mock('@/lib/services.ts',()=>({default:{}}));
import {assetItems,defaultAssetPreferences,assetTotals,type AssetItem} from '@/lib/asset-tools.ts';
const item=(value:string|null,hidden=false,excluded=false):AssetItem=>({key:'cash:a',id:'a',name:'账户',category:1,currency:'CNY',balance:value,value,href:'',portfolio:false,hidden,excluded,unrealizedPnl:'0'});
import type {WealthSummary,InvestmentAccount} from '@/models/investment.ts';

describe('hidden account totals',()=>{
 it('includes hidden assets and debts but obeys the independent exclusion setting',()=>{
  expect(assetTotals([item('100',true),item('-30',true),item('500',false,true)])).toEqual({assets:'100',liabilities:'30',net:'70',missing:0});
 });
 it('keeps known amounts and counts hidden unknown valuations',()=>{
  expect(assetTotals([item('100'),item(null,true)])).toMatchObject({assets:'100',liabilities:'0',net:'100',missing:1});
  expect(assetTotals([item('100'),item(null,true,true)])).toMatchObject({net:'100',missing:0});
 });
 it('retains the known positions within an incomplete account through the top level',()=>{
  const summary={cashAccounts:[],positions:[{accountId:'p',quantity:'1',marketValue:'12.34',unrealizedPnl:'2.34'},{accountId:'p',quantity:'1',marketValue:'',unrealizedPnl:null},{accountId:'p',quantity:'1',marketValue:null,profile:{excludeFromTotal:true}}]} as unknown as WealthSummary;
  const rows=assetItems(summary,[{id:'p',name:'组合',kind:'EXCHANGE'} as InvestmentAccount],{},defaultAssetPreferences());
  expect(rows[0]?.valuation).toEqual({value:'12.34',known:1,missing:1});
  expect(assetTotals([...rows,item('-3')])).toEqual({assets:'12.34',liabilities:'3',net:'9.34',missing:1});
 });
 it('keeps all unknown totals unavailable, while an empty scope is zero',()=>{
  expect(assetTotals([item(''),item(null)])).toEqual({assets:null,liabilities:null,net:null,missing:2});
  expect(assetTotals([])).toEqual({assets:'0',liabilities:'0',net:'0',missing:0});
 });
});
