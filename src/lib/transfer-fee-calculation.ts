import Decimal from 'decimal.js';
import { TRANSACTION_MAX_AMOUNT } from '@/consts/transaction.ts';

const Money = Decimal.clone({ precision: 40 });
const totalPattern = /^(0|[1-9]\d{0,12})(\.\d{1,2})?$/;
const feePattern = /^-?(0|[1-9]\d{0,12})(\.\d{1,2})?$/;

// The stored transfer excludes the separately recorded fee. Both amounts here
// are in the same currency and are converted to integer minor units only after validation.
export function transferPrincipalFromTotal(total: string, fee: string): number {
    if (!totalPattern.test(total) || !feePattern.test(fee)) throw new Error('请输入有效的扣款总额和手续费，最多两位小数。');
    const totalMinor = new Money(total).mul(100);
    const principalMinor = totalMinor.minus(new Money(fee).mul(100));
    if (totalMinor.lte(0) || totalMinor.gt(TRANSACTION_MAX_AMOUNT) || principalMinor.lte(0) || principalMinor.gt(TRANSACTION_MAX_AMOUNT)) {
        throw new Error('扣款总额与计算后的转账金额须大于零且不能超过金额上限。');
    }
    return principalMinor.toNumber();
}
