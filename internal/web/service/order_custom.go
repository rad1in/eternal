package service

import (
	"strconv"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func validOrderKind(kind string) bool {
	return kind == model.OrderKindNew || kind == model.OrderKindRenew || kind == model.OrderKindAddon
}

// offerFor resolves what an order sells: a stored plan, or with planId 0 the
// calculator offer of the given size. Only the calculator can sell an add-on.
func (s *OrderService) offerFor(cfg *SalesConfig, planId int, kind string, gb, days int) (*model.Plan, error) {
	if planId != 0 {
		if kind == model.OrderKindAddon {
			return nil, salesErr(ErrInvalidKind)
		}
		plan, err := s.plans.GetPlan(planId)
		if err != nil || !plan.Enabled {
			return nil, salesErr(ErrPlanUnavailable)
		}
		return plan, nil
	}
	c := cfg.Custom
	if !c.Enabled {
		return nil, salesErr(ErrPlanUnavailable)
	}
	if !c.sizeAllowed(kind, gb, days) {
		return nil, salesErr(ErrCustomInvalid)
	}
	return &model.Plan{
		Name: strconv.Itoa(gb) + " GB / " + strconv.Itoa(days) + " d", Price: int64(gb)*c.PricePerGB + int64(days)*c.PricePerDay,
		Currency: c.Currency, TotalGB: int64(gb) << 30, DurationDays: days, LimitIP: c.LimitIP,
		TemplateId: c.TemplateId, Enabled: true,
	}, nil
}

// sizeAllowed: a new or renewed service needs both a quota and a duration in
// range; an add-on may add just one of them.
func (c CustomOfferConfig) sizeAllowed(kind string, gb, days int) bool {
	gbOK := gb >= c.MinGB && gb <= c.MaxGB
	daysOK := days >= c.MinDays && days <= c.MaxDays
	if kind != model.OrderKindAddon {
		return gbOK && daysOK
	}
	return (gb == 0 || gbOK) && (days == 0 || daysOK) && gb+days > 0
}

// checkOpenOrders keeps one Telegram account from piling up unpaid orders.
func (s *OrderService) checkOpenOrders(tgId int64) error {
	if tgId == 0 {
		return nil
	}
	var open int64
	err := database.GetDB().Model(&model.Order{}).
		Where("tg_id = ? AND status IN ?", tgId, []string{model.OrderStatusPending, model.OrderStatusReview}).
		Count(&open).Error
	if err != nil {
		return err
	}
	if open >= maxOpenOrdersPerTelegram {
		return salesErr(ErrRateLimited)
	}
	return nil
}

// checkAddon refuses an add-on the client cannot take: quota on an unlimited
// client or days on one that never expires would change nothing it pays for.
func (s *OrderService) checkAddon(email string, plan *model.Plan) error {
	rec, err := s.clients.GetRecordByEmail(nil, email)
	if err != nil {
		return salesErr(ErrEmailUnknown)
	}
	if (plan.TotalGB > 0 && rec.TotalGB == 0) || (plan.DurationDays > 0 && rec.ExpiryTime == 0) {
		return salesErr(ErrAddonUnsupported)
	}
	return nil
}
