package models

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

func SeedRentFlowData(db *gorm.DB) error {
	if db == nil {
		return errors.New("ยังไม่ได้เชื่อมต่อฐานข้อมูล")
	}

	defaultTenant := RentFlowTenant{
		ID:           "tenant_fulltank",
		OwnerEmail:   "owner@rentflow.local",
		ShopName:     "Fulltank Garage",
		DomainSlug:   "fulltank",
		PublicDomain: "fulltank.rentflow.com",
		Status:       "active",
		BookingMode:  "payment",
		Plan:         "starter",
	}
	if err := db.Where("id = ?", defaultTenant.ID).Assign(defaultTenant).FirstOrCreate(&RentFlowTenant{}).Error; err != nil {
		return err
	}

	branches := []RentFlowBranch{
		{
			TenantID:        defaultTenant.ID,
			ID:              "suphanburi-city",
			Name:            "สาขาสุพรรณบุรี (ในเมือง)",
			Address:         "ถนนนางพิม ตำบลท่าพี่เลี้ยง อำเภอเมืองสุพรรณบุรี สุพรรณบุรี",
			Phone:           "035-000-111",
			LocationID:      "bangkok",
			Type:            "storefront",
			DisplayOrder:    1,
			Lat:             14.4742,
			Lng:             100.1177,
			OpenTime:        "08:00",
			CloseTime:       "20:00",
			PickupAvailable: true,
			ReturnAvailable: true,
			IsActive:        true,
		},
		{
			TenantID:        defaultTenant.ID,
			ID:              "bangkok-ratchada",
			Name:            "สาขากรุงเทพฯ (รัชดา)",
			Address:         "ถนนรัชดาภิเษก แขวงดินแดง เขตดินแดง กรุงเทพมหานคร",
			Phone:           "02-000-222",
			LocationID:      "bangkok",
			Type:            "storefront",
			DisplayOrder:    2,
			Lat:             13.7736,
			Lng:             100.5731,
			OpenTime:        "08:00",
			CloseTime:       "22:00",
			PickupAvailable: true,
			ReturnAvailable: true,
			IsActive:        true,
		},
		{
			TenantID:        defaultTenant.ID,
			ID:              "don-mueang-airport",
			Name:            "สนามบินดอนเมือง (DMK)",
			Address:         "222 ถนนวิภาวดีรังสิต แขวงสนามบิน เขตดอนเมือง กรุงเทพมหานคร",
			Phone:           "02-000-333",
			LocationID:      "bangkok",
			Type:            "airport",
			DisplayOrder:    3,
			Lat:             13.9126,
			Lng:             100.6070,
			OpenTime:        "06:00",
			CloseTime:       "23:00",
			PickupAvailable: true,
			ReturnAvailable: true,
			ExtraFee:        300,
			IsActive:        true,
		},
		{
			TenantID:        defaultTenant.ID,
			ID:              "suvarnabhumi-airport",
			Name:            "สนามบินสุวรรณภูมิ (BKK)",
			Address:         "999 หมู่ 1 ตำบลหนองปรือ อำเภอบางพลี สมุทรปราการ",
			Phone:           "02-000-444",
			LocationID:      "bangkok",
			Type:            "airport",
			DisplayOrder:    4,
			Lat:             13.6900,
			Lng:             100.7501,
			OpenTime:        "06:00",
			CloseTime:       "23:00",
			PickupAvailable: true,
			ReturnAvailable: true,
			ExtraFee:        300,
			IsActive:        true,
		},
	}

	for _, branch := range branches {
		if err := db.Where("id = ?", branch.ID).Assign(branch).FirstOrCreate(&RentFlowBranch{}).Error; err != nil {
			return err
		}
	}

	now := time.Now()
	cars := []RentFlowCar{
		{
			TenantID:     defaultTenant.ID,
			ID:           "bmw-320d-m-sport",
			Name:         "BMW 320d M Sport",
			Brand:        "BMW",
			Model:        "320d M Sport",
			Year:         2024,
			Type:         "Sedan",
			Seats:        5,
			Transmission: "Auto",
			Fuel:         "Gasoline",
			PricePerDay:  1290,
			UnitCount:    3,
			Description:  "ซีดานขับสบาย เหมาะกับการใช้งานในเมืองและเดินทางต่างจังหวัดแบบคล่องตัว",
			LocationID:   "bangkok",
			Status:       "available",
			IsAvailable:  true,
			CreatedAt:    now,
			UpdatedAt:    now,
		},
		{
			TenantID:     defaultTenant.ID,
			ID:           "bmw-330e-m-sport",
			Name:         "BMW 330e M Sport",
			Brand:        "BMW",
			Model:        "330e M Sport",
			Year:         2024,
			Type:         "Sedan",
			Seats:        5,
			Transmission: "Auto",
			Fuel:         "Hybrid",
			PricePerDay:  1490,
			UnitCount:    2,
			Description:  "ปลั๊กอินไฮบริดที่สมดุลระหว่างความประหยัดและสมรรถนะ",
			LocationID:   "chiang-mai",
			Status:       "available",
			IsAvailable:  true,
		},
		{
			TenantID:     defaultTenant.ID,
			ID:           "bmw-x3-m50",
			Name:         "BMW X3 M50",
			Brand:        "BMW",
			Model:        "X3 M50",
			Year:         2025,
			Type:         "SUV",
			Seats:        5,
			Transmission: "Auto",
			Fuel:         "Gasoline",
			PricePerDay:  1990,
			UnitCount:    2,
			Description:  "SUV สมรรถนะสูง เหมาะกับทริปยาวและการขับขึ้นเขา",
			LocationID:   "phuket",
			Status:       "available",
			IsAvailable:  true,
		},
		{
			TenantID:     defaultTenant.ID,
			ID:           "bmw-i5-edrive40-m-sport",
			Name:         "BMW i5 eDrive40 M Sport",
			Brand:        "BMW",
			Model:        "i5 eDrive40 M Sport",
			Year:         2025,
			Type:         "Sedan",
			Seats:        5,
			Transmission: "Auto",
			Fuel:         "EV",
			PricePerDay:  1590,
			UnitCount:    2,
			Description:  "ซีดานไฟฟ้าล้วน ขับเงียบ นุ่ม และอัดแน่นด้วยเทคโนโลยี",
			LocationID:   "pattaya",
			Status:       "available",
			IsAvailable:  true,
		},
		{
			TenantID:     defaultTenant.ID,
			ID:           "bmw-i5-m60-xdrive",
			Name:         "BMW i5 M60 xDrive",
			Brand:        "BMW",
			Model:        "i5 M60 xDrive",
			Year:         2025,
			Type:         "Sedan",
			Seats:        5,
			Transmission: "Auto",
			Fuel:         "EV",
			PricePerDay:  1790,
			UnitCount:    2,
			Description:  "ซีดานไฟฟ้าแรงจัด ตอบโจทย์ลูกค้าที่ต้องการรถพรีเมียม",
			LocationID:   "bangkok",
			Status:       "available",
			IsAvailable:  true,
		},
		{
			TenantID:     defaultTenant.ID,
			ID:           "bmw-i7-xdrive60-m-sport",
			Name:         "BMW i7 xDrive60 M Sport",
			Brand:        "BMW",
			Model:        "i7 xDrive60 M Sport",
			Year:         2025,
			Type:         "Sedan",
			Seats:        5,
			Transmission: "Auto",
			Fuel:         "EV",
			PricePerDay:  1890,
			UnitCount:    1,
			Description:  "แฟลกชิพซีดานหรู เหมาะกับลูกค้าที่ต้องการประสบการณ์ระดับพรีเมียม",
			LocationID:   "chiang-mai",
			Status:       "available",
			IsAvailable:  true,
		},
	}

	for _, car := range cars {
		if err := db.Where("id = ?", car.ID).Assign(car).FirstOrCreate(&RentFlowCar{}).Error; err != nil {
			return err
		}
	}

	if err := backfillRentFlowDefaultTenant(db, defaultTenant.ID); err != nil {
		return err
	}

	return nil
}

func backfillRentFlowDefaultTenant(db *gorm.DB, tenantID string) error {
	modelsToBackfill := []interface{}{
		&RentFlowBranch{},
		&RentFlowCar{},
		&RentFlowCarImage{},
		&RentFlowBooking{},
		&RentFlowPayment{},
		&RentFlowNotification{},
		&RentFlowReview{},
	}

	for _, model := range modelsToBackfill {
		if err := db.Model(model).
			Where("tenant_id = '' OR tenant_id IS NULL").
			Update("tenant_id", tenantID).Error; err != nil {
			return err
		}
	}

	if err := db.Model(&RentFlowCar{}).
		Where("status = '' OR status IS NULL").
		Update("status", "available").Error; err != nil {
		return err
	}
	if err := db.Model(&RentFlowCar{}).
		Where("unit_count <= 0 OR unit_count IS NULL").
		Update("unit_count", 1).Error; err != nil {
		return err
	}
	if err := db.Model(&RentFlowTenant{}).
		Where("booking_mode = '' OR booking_mode IS NULL").
		Update("booking_mode", "payment").Error; err != nil {
		return err
	}

	if err := db.Model(&RentFlowBranch{}).
		Where("type = '' OR type IS NULL").
		Update("type", "storefront").Error; err != nil {
		return err
	}
	if err := db.Model(&RentFlowBranch{}).
		Where("display_order = 0 OR display_order IS NULL").
		Update("display_order", 1).Error; err != nil {
		return err
	}

	return nil
}
