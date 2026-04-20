package models

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

func SeedRentFlowData(db *gorm.DB) error {
	if db == nil {
		return errors.New("database is nil")
	}

	branches := []RentFlowBranch{
		{
			ID:         "suphanburi-city",
			Name:       "สาขาสุพรรณบุรี (ในเมือง)",
			Address:    "ถนนนางพิม ตำบลท่าพี่เลี้ยง อำเภอเมืองสุพรรณบุรี สุพรรณบุรี",
			Phone:      "035-000-111",
			LocationID: "bangkok",
			Lat:        14.4742,
			Lng:        100.1177,
			OpenTime:   "08:00",
			CloseTime:  "20:00",
			IsActive:   true,
		},
		{
			ID:         "bangkok-ratchada",
			Name:       "สาขากรุงเทพฯ (รัชดา)",
			Address:    "ถนนรัชดาภิเษก แขวงดินแดง เขตดินแดง กรุงเทพมหานคร",
			Phone:      "02-000-222",
			LocationID: "bangkok",
			Lat:        13.7736,
			Lng:        100.5731,
			OpenTime:   "08:00",
			CloseTime:  "22:00",
			IsActive:   true,
		},
		{
			ID:         "don-mueang-airport",
			Name:       "สนามบินดอนเมือง (DMK)",
			Address:    "222 ถนนวิภาวดีรังสิต แขวงสนามบิน เขตดอนเมือง กรุงเทพมหานคร",
			Phone:      "02-000-333",
			LocationID: "bangkok",
			Lat:        13.9126,
			Lng:        100.6070,
			OpenTime:   "06:00",
			CloseTime:  "23:00",
			IsActive:   true,
		},
		{
			ID:         "suvarnabhumi-airport",
			Name:       "สนามบินสุวรรณภูมิ (BKK)",
			Address:    "999 หมู่ 1 ตำบลหนองปรือ อำเภอบางพลี สมุทรปราการ",
			Phone:      "02-000-444",
			LocationID: "bangkok",
			Lat:        13.6900,
			Lng:        100.7501,
			OpenTime:   "06:00",
			CloseTime:  "23:00",
			IsActive:   true,
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
			ImageURL:     "/cosySec.webp",
			ImagesCSV:    "/cosySec.webp,/cosySec1.webp",
			Description:  "ซีดานขับสบาย เหมาะกับการใช้งานในเมืองและเดินทางต่างจังหวัดแบบคล่องตัว",
			LocationID:   "bangkok",
			IsAvailable:  true,
			CreatedAt:    now,
			UpdatedAt:    now,
		},
		{
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
			ImageURL:     "/cosySec1.webp",
			ImagesCSV:    "/cosySec1.webp,/cosySec.webp",
			Description:  "ปลั๊กอินไฮบริดที่สมดุลระหว่างความประหยัดและสมรรถนะ",
			LocationID:   "chiang-mai",
			IsAvailable:  true,
		},
		{
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
			ImageURL:     "/cosySec2.webp",
			ImagesCSV:    "/cosySec2.webp,/cosySec3.webp",
			Description:  "SUV สมรรถนะสูง เหมาะกับทริปยาวและการขับขึ้นเขา",
			LocationID:   "phuket",
			IsAvailable:  true,
		},
		{
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
			ImageURL:     "/cosySec3.webp",
			ImagesCSV:    "/cosySec3.webp,/cosySec4.webp",
			Description:  "ซีดานไฟฟ้าล้วน ขับเงียบ นุ่ม และอัดแน่นด้วยเทคโนโลยี",
			LocationID:   "pattaya",
			IsAvailable:  true,
		},
		{
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
			ImageURL:     "/cosySec4.webp",
			ImagesCSV:    "/cosySec4.webp,/cosySec5.webp",
			Description:  "ซีดานไฟฟ้าแรงจัด ตอบโจทย์ลูกค้าที่ต้องการรถพรีเมียม",
			LocationID:   "bangkok",
			IsAvailable:  true,
		},
		{
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
			ImageURL:     "/cosySec5.webp",
			ImagesCSV:    "/cosySec5.webp,/cosySec4.webp",
			Description:  "แฟลกชิพซีดานหรู เหมาะกับลูกค้าที่ต้องการประสบการณ์ระดับพรีเมียม",
			LocationID:   "chiang-mai",
			IsAvailable:  true,
		},
	}

	for _, car := range cars {
		if err := db.Where("id = ?", car.ID).Assign(car).FirstOrCreate(&RentFlowCar{}).Error; err != nil {
			return err
		}
	}

	return nil
}
