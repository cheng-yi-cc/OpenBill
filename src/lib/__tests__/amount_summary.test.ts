import {describe,it,expect} from 'vitest';
import {ledgerMoney} from '@/lib/ledger-display.ts';
import {summarizeAmounts,combineAmountSummaries} from '@/lib/amount-summary.ts';

describe('known amount subtotals',()=>{
 it('preserves decimal precision and distinguishes zero from unknown',()=>{
  expect(summarizeAmounts(['0.1','0.2',null,'',undefined])).toEqual({value:'0.3',known:2,missing:3});
  expect(summarizeAmounts(['0',null])).toEqual({value:'0',known:1,missing:1});
  expect(summarizeAmounts([null,' '])).toEqual({value:null,known:0,missing:2});
  expect(ledgerMoney('')).toBe('—');
  expect(summarizeAmounts([])).toEqual({value:'0',known:0,missing:0});
 });
 it('does not mistake an empty subgroup for a known amount',()=>{
  expect(combineAmountSummaries([summarizeAmounts([]),summarizeAmounts([null])])).toEqual({value:null,known:0,missing:1});
  expect(combineAmountSummaries([summarizeAmounts(['0.3',null]),summarizeAmounts(['-0.1',null])])).toEqual({value:'0.2',known:2,missing:2});
 });
});
