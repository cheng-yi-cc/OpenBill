export interface LedgerBook {
    id: string;
    name: string;
    icon: string;
    displayOrder: number;
    isDefault: boolean;
    archived: boolean;
    hidden: boolean;
    showTransfers: boolean;
    showInvestments: boolean;
    hiddenCategoryIds?: string[];
    categoryScopeRevision?: number;
}
