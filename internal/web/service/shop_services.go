package service

import (
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

// ShopService is one account a Telegram buyer owns, with its usage. It is
// what "my services" shows in the bot and in the Mini App.
type ShopService struct {
	Id              int    `json:"id" example:"12"`
	Email           string `json:"email" example:"alice"`
	Enable          bool   `json:"enable" example:"true"`
	Up              int64  `json:"up" example:"1048576"`
	Down            int64  `json:"down" example:"2097152"`
	Total           int64  `json:"total" example:"53687091200"`
	ExpiryTime      int64  `json:"expiryTime" example:"1735689600000"`
	SubscriptionURL string `json:"subscriptionUrl"`
}

// OwnedServices lists the clients bound to a Telegram account, with live
// usage. host names the machine the subscription server is reached on.
func (s *OrderService) OwnedServices(tgId int64, host string) ([]ShopService, error) {
	out := []ShopService{}
	if tgId <= 0 {
		return out, nil
	}
	records, err := s.clients.GetRecordsByTgID(tgId)
	if err != nil || len(records) == 0 {
		return out, err
	}
	emails := make([]string, len(records))
	for i, r := range records {
		emails[i] = r.Email
	}
	var traffic []xray.ClientTraffic
	if err := database.GetDB().Where("email IN ?", emails).Find(&traffic).Error; err != nil {
		return nil, err
	}
	byEmail := make(map[string]xray.ClientTraffic, len(traffic))
	for _, tr := range traffic {
		byEmail[tr.Email] = tr
	}
	for _, r := range records {
		tr := byEmail[r.Email]
		out = append(out, ShopService{
			Id: r.Id, Email: r.Email, Enable: r.Enable && (tr.Email == "" || tr.Enable), Up: tr.Up, Down: tr.Down,
			Total: r.TotalGB, ExpiryTime: r.ExpiryTime, SubscriptionURL: s.subscriptionURL(r.Email, host),
		})
	}
	return out, nil
}
