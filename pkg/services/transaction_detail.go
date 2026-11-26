package services

import (
	"fmt"
	"strconv"

	"github.com/mayswind/ezbookkeeping/pkg/core"
	"github.com/mayswind/ezbookkeeping/pkg/models"
	"xorm.io/xorm"
)

func validateTransactionDetail(sess *xorm.Session, tx, old *models.Transaction) error {
	if len(tx.LinkedTransactionIds) > 20 {
		return investmentError("最多关联20笔账单")
	}
	seen := map[int64]bool{}
	for _, raw := range tx.LinkedTransactionIds {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 || id == tx.TransactionId || seen[id] {
			return investmentError("关联账单无效")
		}
		seen[id] = true
		found, err := sess.Where("uid=? AND transaction_id=? AND deleted=?", tx.Uid, id, false).Exist(&models.Transaction{})
		if err != nil {
			return err
		}
		if !found {
			previous := false
			if old != nil {
				for _, v := range old.LinkedTransactionIds {
					if v == raw {
						previous = true
						break
					}
				}
			}
			if !previous {
				return investmentError("关联账单不存在")
			}
		}
	}
	return nil
}

type TransactionRelations struct {
	IDs []string `json:"ids"`
}

func (s *TransactionService) Relations(c core.Context, uid int64, raw string) (*TransactionRelations, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return nil, investmentError("账单无效")
	}
	tx, err := s.GetTransactionByTransactionId(c, uid, id)
	if err != nil {
		return nil, err
	}
	result := &TransactionRelations{IDs: []string{}}
	seen := map[string]bool{}
	add := func(v string) {
		if v != "" && v != "0" && v != raw && !seen[v] {
			seen[v] = true
			result.IDs = append(result.IDs, v)
		}
	}
	for _, v := range tx.LinkedTransactionIds {
		add(v)
	}
	sess := s.UserDataDB(uid).NewSession(c)
	defer sess.Close()
	var rows []*models.Transaction
	if err = sess.Where("uid=? AND deleted=? AND linked_transaction_ids LIKE ?", uid, false, "%\""+raw+"\"%").Find(&rows); err != nil {
		return nil, err
	}
	for _, r := range rows {
		add(fmt.Sprint(r.TransactionId))
	}
	var refunds []*models.Refund
	if err = sess.Where("uid=? AND deleted=? AND (expense_id=? OR transaction_id=?)", uid, false, id, id).Find(&refunds); err != nil {
		return nil, err
	}
	for _, refund := range refunds {
		if refund.ExpenseId == id {
			add(fmt.Sprint(refund.TransactionId))
		}
		if refund.TransactionId == id {
			add(fmt.Sprint(refund.ExpenseId))
		}
	}
	return result, nil
}
