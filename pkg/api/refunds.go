package api

import (
	"strconv"

	"github.com/mayswind/ezbookkeeping/pkg/core"
	"github.com/mayswind/ezbookkeeping/pkg/errs"
	"github.com/mayswind/ezbookkeeping/pkg/services"
)

type RefundsApi struct{}

var Refunds = &RefundsApi{}

func (a *RefundsApi) actor(c *core.WebContext) (services.RefundActor, *errs.Error) {
	zone, err := c.GetClientTimezone()
	if err != nil {
		return services.RefundActor{}, errs.ErrClientTimezoneOffsetInvalid
	}
	user, err := services.Users.GetUserById(c, c.GetCurrentUid())
	if err != nil {
		return services.RefundActor{}, errs.Or(err, errs.ErrUserNotFound)
	}
	return services.RefundActor{User: user, ClientTimezone: zone}, nil
}

func (a *RefundsApi) List(c *core.WebContext) (any, *errs.Error) {
	id, err := strconv.ParseInt(c.Query("expenseId"), 10, 64)
	if err != nil || id <= 0 {
		return nil, errs.ErrTransactionIdInvalid
	}
	return investmentResponse(services.Investments.GetRefunds(c, c.GetCurrentUid(), id))
}

func (a *RefundsApi) Create(c *core.WebContext) (any, *errs.Error) {
	var input services.RefundInput
	if err := investmentBind(c, &input); err != nil {
		return nil, err
	}
	if input.ExpectedOriginalAmount == "" || input.ExpectedRefundedAmount == "" {
		return nil, errs.ErrOperationFailed
	}
	actor, err := a.actor(c)
	if err != nil {
		return nil, err
	}
	return investmentResponse(services.Investments.CreateRefund(c, c.GetCurrentUid(), actor, input))
}

func (a *RefundsApi) Update(c *core.WebContext) (any, *errs.Error) {
	var input struct {
		Id string `json:"id"`
		services.RefundEditInput
	}
	if err := investmentBind(c, &input); err != nil {
		return nil, err
	}
	actor, err := a.actor(c)
	if err != nil {
		return nil, err
	}
	return investmentResponse(services.Investments.UpdateRefund(c, c.GetCurrentUid(), actor, input.Id, input.RefundEditInput))
}

func (a *RefundsApi) UpdateExcess(c *core.WebContext) (any, *errs.Error) {
	var input struct {
		Id string `json:"id"`
		services.RefundExcessEditInput
	}
	if err := investmentBind(c, &input); err != nil {
		return nil, err
	}
	actor, err := a.actor(c)
	if err != nil {
		return nil, err
	}
	return investmentResponse(services.Investments.UpdateRefundExcess(c, c.GetCurrentUid(), actor, input.Id, input.RefundExcessEditInput))
}

func (a *RefundsApi) Delete(c *core.WebContext) (any, *errs.Error) {
	var input struct {
		Id      string `json:"id"`
		Version int64  `json:"version"`
	}
	if err := investmentBind(c, &input); err != nil {
		return nil, err
	}
	actor, err := a.actor(c)
	if err != nil {
		return nil, err
	}
	return investmentResponse(services.Investments.DeleteRefund(c, c.GetCurrentUid(), actor, input.Id, input.Version))
}

func (a *RefundsApi) DeleteExpense(c *core.WebContext) (any, *errs.Error) {
	var input struct {
		ExpenseId string `json:"expenseId"`
	}
	if err := investmentBind(c, &input); err != nil {
		return nil, err
	}
	id, parseErr := strconv.ParseInt(input.ExpenseId, 10, 64)
	if parseErr != nil || id <= 0 {
		return nil, errs.ErrTransactionIdInvalid
	}
	actor, err := a.actor(c)
	if err != nil {
		return nil, err
	}
	if err := services.Investments.DeleteExpenseAndRefunds(c, c.GetCurrentUid(), actor, id); err != nil {
		return nil, errs.Or(err, errs.ErrOperationFailed)
	}
	return true, nil
}
