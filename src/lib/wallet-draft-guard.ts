/** Fields the wallet event cannot yet persist from the ordinary entry form. */
export function unsupportedWalletDraftFields(draft: {
    tagIds: string[]; pictures?: unknown[]; reimbursementAccountId: string; locationName: string; geoLocation?: unknown;
    excludeFromStatistics: boolean; excludeFromBudget: boolean; hideAmount: boolean; discountAmount: string;
    sourceAmount: number; sourceCurrency: string; walletCurrency: string;
    supportsWalletReportingFlags?: boolean; supportsWalletLocationName?: boolean; supportsWalletTags?: boolean;
    supportsWalletDiscount?: boolean; supportsWalletPictures?: boolean;
}): string[] {
    const fields: string[] = [];
    const hasDiscount = Boolean(draft.discountAmount && !/^0+(?:\.0+)?$/.test(draft.discountAmount));
    if (!draft.supportsWalletTags && draft.tagIds.length) fields.push('标签');
    if (!draft.supportsWalletPictures && draft.pictures?.length) fields.push('附件');
    if (draft.reimbursementAccountId && draft.reimbursementAccountId !== '0') fields.push('报销账户');
    if ((!draft.supportsWalletLocationName && draft.locationName) || draft.geoLocation) fields.push('地点');
    if (!draft.supportsWalletReportingFlags && (draft.excludeFromStatistics || draft.excludeFromBudget)) fields.push('收支或预算设置');
    if (!draft.supportsWalletReportingFlags && draft.hideAmount) fields.push('隐藏金额');
    if (!draft.supportsWalletDiscount && hasDiscount) fields.push('优惠金额');
    if ((draft.sourceAmount || (draft.supportsWalletDiscount && hasDiscount)) && draft.sourceCurrency !== draft.walletCurrency) fields.push('不同币种的已输入金额');
    return fields;
}
