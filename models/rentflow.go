package models

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

type RentFlowUser struct {
	ID           string         `gorm:"primaryKey;size:40" json:"id"`
	GoogleSub    *string        `gorm:"size:120;uniqueIndex" json:"-"`
	Username     string         `gorm:"size:80;uniqueIndex" json:"username,omitempty"`
	FirstName    string         `gorm:"size:80" json:"firstName,omitempty"`
	LastName     string         `gorm:"size:80" json:"lastName,omitempty"`
	Name         string         `gorm:"size:150;not null" json:"name"`
	Email        string         `gorm:"size:150;uniqueIndex;not null" json:"email"`
	Phone        string         `gorm:"size:30" json:"phone,omitempty"`
	AvatarURL    string         `gorm:"size:500" json:"avatarUrl,omitempty"`
	PasswordHash string         `gorm:"size:255" json:"-"`
	CreatedAt    time.Time      `json:"createdAt"`
	UpdatedAt    time.Time      `json:"updatedAt"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

func (RentFlowUser) TableName() string {
	return "rentflow_users"
}

type RentFlowBranch struct {
	ID         string    `gorm:"primaryKey;size:60" json:"id"`
	Name       string    `gorm:"size:150;not null" json:"name"`
	Address    string    `gorm:"size:255;not null" json:"address"`
	Phone      string    `gorm:"size:30" json:"phone,omitempty"`
	LocationID string    `gorm:"size:40;index;not null" json:"locationId,omitempty"`
	Lat        float64   `json:"lat,omitempty"`
	Lng        float64   `json:"lng,omitempty"`
	OpenTime   string    `gorm:"size:10" json:"openTime,omitempty"`
	CloseTime  string    `gorm:"size:10" json:"closeTime,omitempty"`
	IsActive   bool      `gorm:"default:true" json:"isActive"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

func (RentFlowBranch) TableName() string {
	return "rentflow_branches"
}

type RentFlowCar struct {
	ID           string         `gorm:"primaryKey;size:80" json:"id"`
	Name         string         `gorm:"size:150;not null" json:"name"`
	Brand        string         `gorm:"size:80;not null" json:"brand"`
	Model        string         `gorm:"size:120;not null" json:"model"`
	Year         int            `gorm:"not null" json:"year"`
	Type         string         `gorm:"size:40;index;not null" json:"type"`
	Seats        int            `gorm:"not null" json:"seats"`
	Transmission string         `gorm:"size:20;not null" json:"transmission"`
	Fuel         string         `gorm:"size:20;not null" json:"fuel"`
	PricePerDay  int64          `gorm:"not null" json:"pricePerDay"`
	ImageURL     string         `gorm:"size:500" json:"imageUrl"`
	ImagesCSV    string         `gorm:"type:text" json:"-"`
	Description  string         `gorm:"type:text" json:"description,omitempty"`
	LocationID   string         `gorm:"size:40;index;not null" json:"locationId,omitempty"`
	IsAvailable  bool           `gorm:"default:true" json:"isAvailable"`
	CreatedAt    time.Time      `json:"createdAt"`
	UpdatedAt    time.Time      `json:"updatedAt"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

func (RentFlowCar) TableName() string {
	return "rentflow_cars"
}

func (c RentFlowCar) Images() []string {
	if strings.TrimSpace(c.ImagesCSV) == "" {
		return nil
	}

	parts := strings.Split(c.ImagesCSV, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

type RentFlowBooking struct {
	ID             string         `gorm:"primaryKey;size:50" json:"id"`
	BookingCode    string         `gorm:"size:30;uniqueIndex;not null" json:"bookingCode"`
	UserID         *string        `gorm:"size:40;index" json:"userId,omitempty"`
	UserEmail      string         `gorm:"size:150;index" json:"-"`
	CarID          string         `gorm:"size:80;index;not null" json:"carId"`
	Status         string         `gorm:"size:20;index;not null" json:"status"`
	PickupDate     time.Time      `gorm:"type:timestamp;not null" json:"pickupDate"`
	ReturnDate     time.Time      `gorm:"type:timestamp;not null" json:"returnDate"`
	PickupLocation string         `gorm:"size:255;not null" json:"pickupLocation"`
	ReturnLocation string         `gorm:"size:255;not null" json:"returnLocation"`
	PickupMethod   string         `gorm:"size:20;not null" json:"pickupMethod"`
	ReturnMethod   string         `gorm:"size:20;not null" json:"returnMethod"`
	TotalDays      int            `gorm:"not null" json:"totalDays"`
	Subtotal       int64          `gorm:"not null" json:"subtotal"`
	ExtraCharge    int64          `gorm:"not null" json:"extraCharge"`
	Discount       int64          `gorm:"not null" json:"discount"`
	TotalAmount    int64          `gorm:"not null" json:"totalAmount"`
	Note           string         `gorm:"type:text" json:"note,omitempty"`
	CustomerName   string         `gorm:"size:150;not null" json:"customerName"`
	CustomerEmail  string         `gorm:"size:150;index;not null" json:"customerEmail"`
	CustomerPhone  string         `gorm:"size:30;not null" json:"customerPhone"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

func (RentFlowBooking) TableName() string {
	return "rentflow_bookings"
}

type RentFlowPayment struct {
	ID            string         `gorm:"primaryKey;size:50" json:"id"`
	BookingID     string         `gorm:"size:50;index;not null" json:"bookingId"`
	Method        string         `gorm:"size:30;not null" json:"method"`
	Status        string         `gorm:"size:20;index;not null" json:"status"`
	Amount        int64          `gorm:"not null" json:"amount"`
	TransactionID string         `gorm:"size:120" json:"transactionId,omitempty"`
	PaymentURL    string         `gorm:"size:500" json:"paymentUrl,omitempty"`
	QRCodeURL     string         `gorm:"size:500" json:"qrCodeUrl,omitempty"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

func (RentFlowPayment) TableName() string {
	return "rentflow_payments"
}

type RentFlowNotification struct {
	ID        string    `gorm:"primaryKey;size:50" json:"id"`
	UserID    *string   `gorm:"size:40;index" json:"-"`
	UserEmail string    `gorm:"size:150;index;not null" json:"-"`
	Title     string    `gorm:"size:180;not null" json:"title"`
	Message   string    `gorm:"type:text;not null" json:"message"`
	IsRead    bool      `gorm:"default:false" json:"isRead"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (RentFlowNotification) TableName() string {
	return "rentflow_notifications"
}

type RentFlowReview struct {
	ID        string         `gorm:"primaryKey;size:50" json:"id"`
	FirstName string         `gorm:"size:80;not null" json:"firstName"`
	LastName  string         `gorm:"size:80;not null" json:"lastName"`
	Rating    int            `gorm:"not null" json:"rating"`
	Comment   string         `gorm:"type:text" json:"comment,omitempty"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (RentFlowReview) TableName() string {
	return "rentflow_reviews"
}
