import { describe, expect, it } from 'vitest';
import { unsupportedWalletDraftFields } from '../wallet-draft-guard.ts';

const empty = () => ({ tagIds: [], pictures: [], reimbursementAccountId: '0', locationName: '', geoLocation: undefined, excludeFromStatistics: false, excludeFromBudget: false, hideAmount: false, discountAmount: '0.00', sourceAmount: 100, sourceCurrency: 'CNY', walletCurrency: 'CNY' });
describe('钱包记账草稿保护', () => {
    it('允许可以完整传递的同币种草稿', () => expect(unsupportedWalletDraftFields(empty())).toEqual([]));
    it('钱包入口支持属性时允许传递独立标记，仍保护其他不支持的内容', () => {
        expect(unsupportedWalletDraftFields({ ...empty(), supportsWalletReportingFlags: true, excludeFromStatistics: true, excludeFromBudget: true, hideAmount: true, tagIds: ['1'] })).toEqual(['标签']);
    });
    it('支持地点名称时仍保护不能带入的地图坐标', () => {
        expect(unsupportedWalletDraftFields({ ...empty(), supportsWalletLocationName: true, locationName: '测试地点' })).toEqual([]);
        expect(unsupportedWalletDraftFields({ ...empty(), supportsWalletLocationName: true, locationName: '测试地点', geoLocation: { latitude: 1, longitude: 2 } })).toEqual(['地点']);
    });
    it('钱包支持标签时允许带入标签但仍保护未开放的附件', () => {
        expect(unsupportedWalletDraftFields({ ...empty(), supportsWalletTags: true, tagIds: ['1'], pictures: [{}] })).toEqual(['附件']);
    });
    it('钱包支持优惠和附件时允许同币种带入，跨币种优惠仍提示拦截', () => {
        expect(unsupportedWalletDraftFields({ ...empty(), supportsWalletDiscount: true, supportsWalletPictures: true, discountAmount: '2.50', pictures: [{}] })).toEqual([]);
        expect(unsupportedWalletDraftFields({ ...empty(), supportsWalletDiscount: true, supportsWalletPictures: true, sourceAmount: 0, discountAmount: '2.50', pictures: [{}], walletCurrency: 'USD' })).toEqual(['不同币种的已输入金额']);
    });
    it('识别不会被钱包事件保存的设置和无法直接转用的金额', () => {
        expect(unsupportedWalletDraftFields({ ...empty(), tagIds: ['1'], pictures: [{}], reimbursementAccountId: '2', locationName: '地点', excludeFromStatistics: true, excludeFromBudget: true, hideAmount: true, discountAmount: '0.01', walletCurrency: 'USD' })).toEqual(['标签', '附件', '报销账户', '地点', '收支或预算设置', '隐藏金额', '优惠金额', '不同币种的已输入金额']);
        expect(unsupportedWalletDraftFields({ ...empty(), sourceAmount: 0, walletCurrency: 'USD' })).toEqual([]);
        expect(unsupportedWalletDraftFields({ ...empty(), geoLocation: { latitude: 1, longitude: 2 } })).toEqual(['地点']);
    });
});
