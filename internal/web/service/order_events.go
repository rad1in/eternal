package service

import (
	"strconv"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

const (
	// A buyer is nudged once about an order that is still unpaid after this.
	orderReminderAfter = 30 * time.Minute
)

func publishOrder(typ eventbus.EventType, o *model.Order) {
	if eventBus == nil {
		return
	}
	eventBus.Publish(eventbus.Event{
		Type: typ, Source: "order-" + strconv.Itoa(o.Id),
		Data: &eventbus.OrderEventData{OrderId: o.Id, TgId: o.TgId}, Timestamp: time.Now(),
	})
}

// SweepUnpaid expires card orders nobody paid in orderExpiry (freeing their
// username) and nudges Telegram buyers once about orders still unpaid after
// orderReminderAfter. Online orders are expired by ReconcileOnline.
func (s *OrderService) SweepUnpaid(now time.Time) error {
	db := database.GetDB()
	var stale []model.Order
	err := db.Where("gateway = ? AND status = ? AND created_at <= ?", GatewayCard, model.OrderStatusPending, now.Add(-orderExpiry).UnixMilli()).
		Find(&stale).Error
	if err != nil {
		return err
	}
	for i := range stale {
		if o, err := s.closeOrder(stale[i].Id, model.OrderStatusCanceled); err != nil {
			logger.Warningf("order %d: expiring: %v", stale[i].Id, err)
		} else if o.Status == model.OrderStatusCanceled {
			publishOrder(eventbus.EventOrderExpired, o)
		}
	}

	var due []model.Order
	err = db.Where("tg_id <> 0 AND status = ? AND reminded_at = 0 AND created_at <= ? AND created_at > ?",
		model.OrderStatusPending, now.Add(-orderReminderAfter).UnixMilli(), now.Add(-orderExpiry).UnixMilli()).
		Find(&due).Error
	if err != nil {
		return err
	}
	for i := range due {
		res := db.Model(&model.Order{}).Where("id = ? AND reminded_at = 0 AND status = ?", due[i].Id, model.OrderStatusPending).
			Update("reminded_at", now.UnixMilli())
		if res.Error == nil && res.RowsAffected == 1 {
			publishOrder(eventbus.EventOrderReminder, &due[i])
		}
	}
	return nil
}

// SalesDay is what one currency took in over a period.
type SalesDay struct {
	Currency string `json:"currency"`
	Orders   int    `json:"orders"`
	Amount   int64  `json:"amount"`
}

// SalesSummary counts paid orders since a moment, per currency, and what is
// still waiting on the admin or the buyer.
type SalesSummary struct {
	Paid    []SalesDay `json:"paid"`
	Pending int64      `json:"pending"`
	Review  int64      `json:"review"`
}

// SalesSummary reports the sales since the given moment.
func (s *OrderService) SalesSummary(since time.Time) (*SalesSummary, error) {
	db := database.GetDB()
	out := &SalesSummary{Paid: []SalesDay{}}
	err := db.Model(&model.Order{}).
		Where("status = ? AND paid_at >= ?", model.OrderStatusPaid, since.UnixMilli()).
		Select("currency, COUNT(*) AS orders, COALESCE(SUM(amount), 0) AS amount").Group("currency").Order("currency").
		Scan(&out.Paid).Error
	if err != nil {
		return nil, err
	}
	if err := db.Model(&model.Order{}).Where("status = ?", model.OrderStatusPending).Count(&out.Pending).Error; err != nil {
		return nil, err
	}
	err = db.Model(&model.Order{}).Where("status = ?", model.OrderStatusReview).Count(&out.Review).Error
	return out, err
}

// OrdersInReview lists the card orders waiting for an admin, oldest first.
func (s *OrderService) OrdersInReview(limit int) ([]model.Order, error) {
	var orders []model.Order
	err := database.GetDB().Where("status = ?", model.OrderStatusReview).Order("id asc").Limit(limit).Find(&orders).Error
	return orders, err
}
