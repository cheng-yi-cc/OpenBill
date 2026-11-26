package models

// Book groups transaction facts. Accounts and their balances remain user-wide.
type Book struct {
	Id                    string   `xorm:"VARCHAR(64) PK" json:"id"`
	Uid                   int64    `xorm:"INDEX NOT NULL" json:"-"`
	Name                  string   `xorm:"VARCHAR(64) NOT NULL" json:"name"`
	Icon                  string   `xorm:"VARCHAR(32) NOT NULL" json:"icon"`
	DisplayOrder          int32    `json:"displayOrder"`
	Archived              bool     `json:"archived"`
	Hidden                bool     `xorm:"NOT NULL DEFAULT 0" json:"hidden"`
	IsDefault             bool     `json:"isDefault"`
	ShowTransfers         bool     `json:"showTransfers"`
	ShowInvestments       bool     `json:"showInvestments"`
	HiddenCategoryIds     []string `xorm:"TEXT JSON" json:"hiddenCategoryIds"`
	CategoryScopeRevision int64    `xorm:"NOT NULL DEFAULT 0" json:"categoryScopeRevision"`
}

type BookCreateRequest struct {
	Name                   string `json:"name" binding:"required,notBlank,max=64"`
	Icon                   string `json:"icon" binding:"max=32"`
	ShowTransfers          *bool  `json:"showTransfers"`
	ShowInvestments        *bool  `json:"showInvestments"`
	HideExistingCategories bool   `json:"hideExistingCategories"`
}

type BookCategoryVisibilityChange struct {
	CategoryId string `json:"categoryId" binding:"required,max=64"`
	Hidden     bool   `json:"hidden"`
}

type BookCategoryVisibilityRequest struct {
	BookId   string                         `json:"bookId" binding:"required,max=64"`
	Revision int64                          `json:"revision" binding:"min=0"`
	Changes  []BookCategoryVisibilityChange `json:"changes" binding:"required,min=1,max=1000,dive"`
}

type BookModifyRequest struct {
	Id              string  `json:"id" binding:"required,max=64"`
	Name            *string `json:"name" binding:"omitempty,notBlank,max=64"`
	Icon            *string `json:"icon" binding:"omitempty,max=32"`
	DisplayOrder    *int32  `json:"displayOrder"`
	Archived        *bool   `json:"archived"`
	Hidden          *bool   `json:"hidden"`
	IsDefault       *bool   `json:"isDefault"`
	ShowTransfers   *bool   `json:"showTransfers"`
	ShowInvestments *bool   `json:"showInvestments"`
}

type BookMoveRequest struct {
	TransactionIds []string `json:"transactionIds" binding:"required,min=1,max=1000"`
	BookId         string   `json:"bookId" binding:"required,max=64"`
}

type BookMoveAllRequest struct {
	SourceBookId string `json:"sourceBookId" binding:"required,max=64"`
	TargetBookId string `json:"targetBookId" binding:"required,max=64"`
}
