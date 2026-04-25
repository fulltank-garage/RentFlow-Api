package models

import (
	"time"

	"gorm.io/gorm"
)

type RentFlowUser struct {
	ID             string         `gorm:"primaryKey;size:40" json:"id"`
	GoogleSub      *string        `gorm:"size:120;uniqueIndex" json:"-"`
	Username       string         `gorm:"size:80;uniqueIndex" json:"username,omitempty"`
	FirstName      string         `gorm:"size:80" json:"firstName,omitempty"`
	LastName       string         `gorm:"size:80" json:"lastName,omitempty"`
	Name           string         `gorm:"size:150;not null" json:"name"`
	Email          string         `gorm:"size:150;uniqueIndex;not null" json:"email"`
	Phone          string         `gorm:"size:30" json:"phone,omitempty"`
	AvatarURL      string         `gorm:"-" json:"avatarUrl,omitempty"`
	AvatarMimeType string         `gorm:"size:80" json:"-"`
	AvatarBlob     []byte         `gorm:"type:bytea" json:"-"`
	PasswordHash   string         `gorm:"size:255" json:"-"`
	Status         string         `gorm:"size:30;index;not null;default:active" json:"status"`
	LockedReason   string         `gorm:"type:text" json:"lockedReason,omitempty"`
	LockedAt       *time.Time     `json:"lockedAt,omitempty"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

func (RentFlowUser) TableName() string {
	return "rentflow_users"
}

type RentFlowTenant struct {
	ID                 string         `gorm:"primaryKey;size:50" json:"id"`
	OwnerUserID        *string        `gorm:"size:40;index" json:"ownerUserId,omitempty"`
	OwnerEmail         string         `gorm:"size:150;index;not null" json:"ownerEmail"`
	ShopName           string         `gorm:"size:150;not null" json:"shopName"`
	DomainSlug         string         `gorm:"size:80;uniqueIndex;not null" json:"domainSlug"`
	PublicDomain       string         `gorm:"size:160;uniqueIndex;not null" json:"publicDomain"`
	LogoURL            string         `gorm:"-" json:"logoUrl,omitempty"`
	LogoMimeType       string         `gorm:"size:80" json:"-"`
	LogoBlob           []byte         `gorm:"type:bytea" json:"-"`
	PromoImageURL      string         `gorm:"-" json:"promoImageUrl,omitempty"`
	PromoImageMimeType string         `gorm:"size:80" json:"-"`
	PromoImageBlob     []byte         `gorm:"type:bytea" json:"-"`
	Status             string         `gorm:"size:30;index;not null;default:active" json:"status"`
	BookingMode        string         `gorm:"size:30;not null;default:payment" json:"bookingMode"`
	Plan               string         `gorm:"size:40;not null;default:starter" json:"plan"`
	LifecycleReason    string         `gorm:"type:text" json:"lifecycleReason,omitempty"`
	ApprovedAt         *time.Time     `json:"approvedAt,omitempty"`
	SuspendedAt        *time.Time     `json:"suspendedAt,omitempty"`
	RejectedAt         *time.Time     `json:"rejectedAt,omitempty"`
	CreatedAt          time.Time      `json:"createdAt"`
	UpdatedAt          time.Time      `json:"updatedAt"`
	DeletedAt          gorm.DeletedAt `gorm:"index" json:"-"`
}

func (RentFlowTenant) TableName() string {
	return "rentflow_tenants"
}

type RentFlowTenantPromoImage struct {
	ID           string    `gorm:"primaryKey;size:80" json:"id"`
	TenantID     string    `gorm:"size:50;index;not null" json:"tenantId"`
	MimeType     string    `gorm:"size:80;not null" json:"-"`
	Blob         []byte    `gorm:"type:bytea;not null" json:"-"`
	DisplayOrder int       `gorm:"index;default:1" json:"displayOrder"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

func (RentFlowTenantPromoImage) TableName() string {
	return "rentflow_tenant_promo_images"
}

type RentFlowPlatformSetting struct {
	Key           string    `gorm:"primaryKey;size:80" json:"key"`
	ImageURL      string    `gorm:"-" json:"imageUrl,omitempty"`
	ImageMimeType string    `gorm:"size:80" json:"-"`
	ImageBlob     []byte    `gorm:"type:bytea" json:"-"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

func (RentFlowPlatformSetting) TableName() string {
	return "rentflow_platform_settings"
}

type RentFlowBranch struct {
	ID              string    `gorm:"primaryKey;size:60" json:"id"`
	TenantID        string    `gorm:"size:50;index" json:"tenantId,omitempty"`
	Name            string    `gorm:"size:150;not null" json:"name"`
	Address         string    `gorm:"size:255;not null" json:"address"`
	Phone           string    `gorm:"size:30" json:"phone,omitempty"`
	LocationID      string    `gorm:"size:40;index;not null" json:"locationId,omitempty"`
	Type            string    `gorm:"size:30;index;default:storefront" json:"type,omitempty"`
	DisplayOrder    int       `gorm:"index;default:1" json:"displayOrder"`
	Lat             float64   `json:"lat,omitempty"`
	Lng             float64   `json:"lng,omitempty"`
	OpenTime        string    `gorm:"size:10" json:"openTime,omitempty"`
	CloseTime       string    `gorm:"size:10" json:"closeTime,omitempty"`
	PickupAvailable bool      `gorm:"default:true" json:"pickupAvailable"`
	ReturnAvailable bool      `gorm:"default:true" json:"returnAvailable"`
	ExtraFee        int64     `gorm:"default:0" json:"extraFee"`
	IsActive        bool      `gorm:"default:true" json:"isActive"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

func (RentFlowBranch) TableName() string {
	return "rentflow_branches"
}

type RentFlowCar struct {
	ID           string         `gorm:"primaryKey;size:80" json:"id"`
	TenantID     string         `gorm:"size:50;index" json:"tenantId,omitempty"`
	Name         string         `gorm:"size:150;not null" json:"name"`
	Brand        string         `gorm:"size:80;not null" json:"brand"`
	Model        string         `gorm:"size:120;not null" json:"model"`
	Year         int            `gorm:"not null" json:"year"`
	Type         string         `gorm:"size:40;index;not null" json:"type"`
	Seats        int            `gorm:"not null" json:"seats"`
	Transmission string         `gorm:"size:20;not null" json:"transmission"`
	Fuel         string         `gorm:"size:20;not null" json:"fuel"`
	PricePerDay  int64          `gorm:"not null" json:"pricePerDay"`
	UnitCount    int            `gorm:"not null;default:1" json:"unitCount"`
	Description  string         `gorm:"type:text" json:"description,omitempty"`
	LocationID   string         `gorm:"size:40;index;not null" json:"locationId,omitempty"`
	Status       string         `gorm:"size:20;index;default:available" json:"status"`
	IsAvailable  bool           `gorm:"default:true" json:"isAvailable"`
	CreatedAt    time.Time      `json:"createdAt"`
	UpdatedAt    time.Time      `json:"updatedAt"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

func (RentFlowCar) TableName() string {
	return "rentflow_cars"
}

type RentFlowCarImage struct {
	ID        string    `gorm:"primaryKey;size:120" json:"id"`
	TenantID  string    `gorm:"size:50;index" json:"tenantId,omitempty"`
	CarID     string    `gorm:"size:80;index;not null" json:"carId"`
	SortOrder int       `gorm:"index;not null;default:0" json:"sortOrder"`
	FileName  string    `gorm:"size:160" json:"fileName,omitempty"`
	MimeType  string    `gorm:"size:80;not null" json:"mimeType"`
	ImageBlob []byte    `gorm:"type:bytea;not null" json:"-"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (RentFlowCarImage) TableName() string {
	return "rentflow_car_images"
}

type RentFlowBooking struct {
	ID             string         `gorm:"primaryKey;size:50" json:"id"`
	TenantID       string         `gorm:"size:50;index" json:"tenantId,omitempty"`
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
	AddonsJSON     string         `gorm:"type:text" json:"addonsJson,omitempty"`
	AddonsTotal    int64          `gorm:"not null;default:0" json:"addonsTotal"`
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
	TenantID      string         `gorm:"size:50;index" json:"tenantId,omitempty"`
	BookingID     string         `gorm:"size:50;index;not null" json:"bookingId"`
	Method        string         `gorm:"size:30;not null" json:"method"`
	Status        string         `gorm:"size:20;index;not null" json:"status"`
	Amount        int64          `gorm:"not null" json:"amount"`
	TransactionID string         `gorm:"size:120" json:"transactionId,omitempty"`
	PaymentURL    string         `gorm:"size:500" json:"paymentUrl,omitempty"`
	QRCodeURL     string         `gorm:"size:500" json:"qrCodeUrl,omitempty"`
	SlipURL       string         `gorm:"-" json:"slipUrl,omitempty"`
	SlipMimeType  string         `gorm:"size:80" json:"-"`
	SlipBlob      []byte         `gorm:"type:bytea" json:"-"`
	VerifiedBy    string         `gorm:"size:50" json:"verifiedBy,omitempty"`
	VerifiedAt    *time.Time     `json:"verifiedAt,omitempty"`
	RefundStatus  string         `gorm:"size:30;index;default:none" json:"refundStatus,omitempty"`
	RefundAmount  int64          `gorm:"default:0" json:"refundAmount,omitempty"`
	PayoutStatus  string         `gorm:"size:30;index;default:pending" json:"payoutStatus,omitempty"`
	SettledAt     *time.Time     `json:"settledAt,omitempty"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

func (RentFlowPayment) TableName() string {
	return "rentflow_payments"
}

type RentFlowNotification struct {
	ID        string    `gorm:"primaryKey;size:50" json:"id"`
	TenantID  string    `gorm:"size:50;index" json:"tenantId,omitempty"`
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

type RentFlowMessageLog struct {
	ID           string    `gorm:"primaryKey;size:50" json:"id"`
	TenantID     string    `gorm:"size:50;index" json:"tenantId,omitempty"`
	Channel      string    `gorm:"size:30;index;not null" json:"channel"`
	Recipient    string    `gorm:"size:180;index;not null" json:"recipient"`
	Subject      string    `gorm:"size:180" json:"subject,omitempty"`
	Body         string    `gorm:"type:text" json:"body,omitempty"`
	Status       string    `gorm:"size:30;index;not null;default:queued" json:"status"`
	ProviderRef  string    `gorm:"size:160" json:"providerRef,omitempty"`
	ErrorMessage string    `gorm:"type:text" json:"errorMessage,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

func (RentFlowMessageLog) TableName() string {
	return "rentflow_message_logs"
}

type RentFlowReview struct {
	ID        string         `gorm:"primaryKey;size:50" json:"id"`
	TenantID  string         `gorm:"size:50;index" json:"tenantId,omitempty"`
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

type RentFlowTenantMember struct {
	ID              string         `gorm:"primaryKey;size:50" json:"id"`
	TenantID        string         `gorm:"size:50;index;not null" json:"tenantId"`
	UserID          string         `gorm:"size:40;index" json:"userId,omitempty"`
	Email           string         `gorm:"size:150;index;not null" json:"email"`
	Name            string         `gorm:"size:150" json:"name,omitempty"`
	Role            string         `gorm:"size:30;index;not null;default:staff" json:"role"`
	PermissionsJSON string         `gorm:"type:text" json:"permissionsJson,omitempty"`
	Status          string         `gorm:"size:30;index;not null;default:active" json:"status"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

func (RentFlowTenantMember) TableName() string {
	return "rentflow_tenant_members"
}

type RentFlowCustomDomain struct {
	ID              string         `gorm:"primaryKey;size:50" json:"id"`
	TenantID        string         `gorm:"size:50;index;not null" json:"tenantId"`
	Domain          string         `gorm:"size:180;uniqueIndex;not null" json:"domain"`
	Status          string         `gorm:"size:30;index;not null;default:pending" json:"status"`
	VerificationTXT string         `gorm:"size:180" json:"verificationTxt,omitempty"`
	VerifiedAt      *time.Time     `json:"verifiedAt,omitempty"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

func (RentFlowCustomDomain) TableName() string {
	return "rentflow_custom_domains"
}

type RentFlowAuditLog struct {
	ID         string    `gorm:"primaryKey;size:50" json:"id"`
	TenantID   string    `gorm:"size:50;index" json:"tenantId,omitempty"`
	ActorID    string    `gorm:"size:50;index" json:"actorId,omitempty"`
	ActorEmail string    `gorm:"size:150;index" json:"actorEmail,omitempty"`
	Action     string    `gorm:"size:120;index;not null" json:"action"`
	Entity     string    `gorm:"size:80;index" json:"entity,omitempty"`
	EntityID   string    `gorm:"size:80;index" json:"entityId,omitempty"`
	Detail     string    `gorm:"type:text" json:"detail,omitempty"`
	IP         string    `gorm:"size:80" json:"ip,omitempty"`
	UserAgent  string    `gorm:"size:500" json:"userAgent,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

func (RentFlowAuditLog) TableName() string {
	return "rentflow_audit_logs"
}

type RentFlowAvailabilityBlock struct {
	ID          string         `gorm:"primaryKey;size:50" json:"id"`
	TenantID    string         `gorm:"size:50;index;not null" json:"tenantId"`
	CarID       string         `gorm:"size:80;index" json:"carId,omitempty"`
	BranchID    string         `gorm:"size:60;index" json:"branchId,omitempty"`
	StartDate   time.Time      `gorm:"type:timestamp;index;not null" json:"startDate"`
	EndDate     time.Time      `gorm:"type:timestamp;index;not null" json:"endDate"`
	BlockType   string         `gorm:"size:40;index;not null;default:maintenance" json:"blockType"`
	BufferHours int            `gorm:"default:0" json:"bufferHours"`
	Reason      string         `gorm:"size:120;not null" json:"reason"`
	Note        string         `gorm:"type:text" json:"note,omitempty"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

func (RentFlowAvailabilityBlock) TableName() string {
	return "rentflow_availability_blocks"
}

type RentFlowPromotion struct {
	ID            string         `gorm:"primaryKey;size:50" json:"id"`
	TenantID      string         `gorm:"size:50;index;not null" json:"tenantId"`
	Code          string         `gorm:"size:60;index;not null" json:"code"`
	Name          string         `gorm:"size:150;not null" json:"name"`
	Description   string         `gorm:"type:text" json:"description,omitempty"`
	DiscountType  string         `gorm:"size:20;not null;default:percent" json:"discountType"`
	DiscountValue int64          `gorm:"not null;default:0" json:"discountValue"`
	StartsAt      *time.Time     `json:"startsAt,omitempty"`
	EndsAt        *time.Time     `json:"endsAt,omitempty"`
	IsActive      bool           `gorm:"default:true" json:"isActive"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

func (RentFlowPromotion) TableName() string {
	return "rentflow_promotions"
}

type RentFlowAddon struct {
	ID          string         `gorm:"primaryKey;size:50" json:"id"`
	TenantID    string         `gorm:"size:50;index;not null" json:"tenantId"`
	Name        string         `gorm:"size:150;not null" json:"name"`
	Description string         `gorm:"type:text" json:"description,omitempty"`
	Price       int64          `gorm:"not null;default:0" json:"price"`
	Unit        string         `gorm:"size:30;not null;default:day" json:"unit"`
	IsActive    bool           `gorm:"default:true" json:"isActive"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

func (RentFlowAddon) TableName() string {
	return "rentflow_addons"
}

type RentFlowLead struct {
	ID            string         `gorm:"primaryKey;size:50" json:"id"`
	TenantID      string         `gorm:"size:50;index;not null" json:"tenantId"`
	Name          string         `gorm:"size:150;not null" json:"name"`
	Email         string         `gorm:"size:150;index" json:"email,omitempty"`
	Phone         string         `gorm:"size:40;index" json:"phone,omitempty"`
	Source        string         `gorm:"size:80;index" json:"source,omitempty"`
	Status        string         `gorm:"size:30;index;not null;default:new" json:"status"`
	InterestedCar string         `gorm:"size:120" json:"interestedCar,omitempty"`
	Note          string         `gorm:"type:text" json:"note,omitempty"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

func (RentFlowLead) TableName() string {
	return "rentflow_leads"
}

type RentFlowLineChannel struct {
	ID                    string         `gorm:"primaryKey;size:50" json:"id"`
	TenantID              string         `gorm:"size:50;uniqueIndex;not null" json:"tenantId"`
	ChannelID             string         `gorm:"size:120;not null" json:"channelId"`
	ChannelSecret         string         `gorm:"type:text;not null" json:"-"`
	AccessToken           string         `gorm:"type:text;not null" json:"-"`
	DisplayName           string         `gorm:"size:180" json:"displayName,omitempty"`
	BasicID               string         `gorm:"size:120" json:"basicId,omitempty"`
	BotUserID             string         `gorm:"size:120" json:"botUserId,omitempty"`
	PictureURL            string         `gorm:"size:500" json:"pictureUrl,omitempty"`
	WebhookURL            string         `gorm:"size:500" json:"webhookUrl,omitempty"`
	Status                string         `gorm:"size:30;index;not null;default:draft" json:"status"`
	LastVerifiedAt        *time.Time     `json:"lastVerifiedAt,omitempty"`
	LastWebhookTestAt     *time.Time     `json:"lastWebhookTestAt,omitempty"`
	LastWebhookTestStatus string         `gorm:"size:30" json:"lastWebhookTestStatus,omitempty"`
	LastError             string         `gorm:"type:text" json:"lastError,omitempty"`
	CreatedAt             time.Time      `json:"createdAt"`
	UpdatedAt             time.Time      `json:"updatedAt"`
	DeletedAt             gorm.DeletedAt `gorm:"index" json:"-"`
}

func (RentFlowLineChannel) TableName() string {
	return "rentflow_line_channels"
}

type RentFlowSupportTicket struct {
	ID               string         `gorm:"primaryKey;size:50" json:"id"`
	TenantID         string         `gorm:"size:50;index;not null" json:"tenantId"`
	Channel          string         `gorm:"size:30;index:idx_rentflow_support_thread,unique;not null" json:"channel"`
	ExternalThreadID string         `gorm:"size:160;index:idx_rentflow_support_thread,unique" json:"externalThreadId,omitempty"`
	Subject          string         `gorm:"size:180;not null" json:"subject"`
	CustomerName     string         `gorm:"size:150" json:"customerName,omitempty"`
	CustomerPhone    string         `gorm:"size:40;index" json:"customerPhone,omitempty"`
	CustomerEmail    string         `gorm:"size:150;index" json:"customerEmail,omitempty"`
	Status           string         `gorm:"size:30;index;not null;default:new" json:"status"`
	Priority         string         `gorm:"size:30;index;not null;default:normal" json:"priority"`
	OwnerEmail       string         `gorm:"size:150;index" json:"ownerEmail,omitempty"`
	BookingID        string         `gorm:"size:50;index" json:"bookingId,omitempty"`
	LastMessage      string         `gorm:"type:text" json:"lastMessage,omitempty"`
	LastMessageAt    *time.Time     `json:"lastMessageAt,omitempty"`
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`
}

func (RentFlowSupportTicket) TableName() string {
	return "rentflow_support_tickets"
}

type RentFlowSupportMessage struct {
	ID          string         `gorm:"primaryKey;size:50" json:"id"`
	TenantID    string         `gorm:"size:50;index;not null" json:"tenantId"`
	TicketID    string         `gorm:"size:50;index;not null" json:"ticketId"`
	FromType    string         `gorm:"size:20;index;not null" json:"fromType"`
	Message     string         `gorm:"type:text;not null" json:"message"`
	IsInternal  bool           `gorm:"default:false" json:"isInternal"`
	Status      string         `gorm:"size:30;index;not null;default:logged" json:"status"`
	ProviderRef string         `gorm:"size:160;index" json:"providerRef,omitempty"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

func (RentFlowSupportMessage) TableName() string {
	return "rentflow_support_messages"
}

type RentFlowBookingOperation struct {
	ID            string         `gorm:"primaryKey;size:50" json:"id"`
	TenantID      string         `gorm:"size:50;index;not null" json:"tenantId"`
	BookingID     string         `gorm:"size:50;index;not null" json:"bookingId"`
	Type          string         `gorm:"size:40;index;not null" json:"type"`
	ChecklistJSON string         `gorm:"type:text" json:"checklistJson,omitempty"`
	Odometer      int64          `gorm:"default:0" json:"odometer,omitempty"`
	FuelLevel     string         `gorm:"size:40" json:"fuelLevel,omitempty"`
	DamageNote    string         `gorm:"type:text" json:"damageNote,omitempty"`
	FineAmount    int64          `gorm:"default:0" json:"fineAmount,omitempty"`
	StaffNote     string         `gorm:"type:text" json:"staffNote,omitempty"`
	CreatedBy     string         `gorm:"size:50;index" json:"createdBy,omitempty"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

func (RentFlowBookingOperation) TableName() string {
	return "rentflow_booking_operations"
}

type RentFlowStorefrontPage struct {
	ID          string     `gorm:"primaryKey;size:50" json:"id"`
	TenantID    string     `gorm:"size:50;index" json:"tenantId,omitempty"`
	Scope       string     `gorm:"size:30;index;not null;default:tenant" json:"scope"`
	Page        string     `gorm:"size:60;index;not null;default:home" json:"page"`
	ThemeJSON   string     `gorm:"type:text" json:"themeJson,omitempty"`
	BlocksJSON  string     `gorm:"type:text" json:"blocksJson,omitempty"`
	IsPublished bool       `gorm:"default:true" json:"isPublished"`
	PublishedAt *time.Time `json:"publishedAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

func (RentFlowStorefrontPage) TableName() string {
	return "rentflow_storefront_pages"
}

type RentFlowPlatformInvoice struct {
	ID        string         `gorm:"primaryKey;size:50" json:"id"`
	TenantID  string         `gorm:"size:50;index;not null" json:"tenantId"`
	Period    string         `gorm:"size:20;index;not null" json:"period"`
	Plan      string         `gorm:"size:40;index;not null" json:"plan"`
	Amount    int64          `gorm:"not null;default:0" json:"amount"`
	Status    string         `gorm:"size:30;index;not null;default:draft" json:"status"`
	IssuedAt  *time.Time     `json:"issuedAt,omitempty"`
	PaidAt    *time.Time     `json:"paidAt,omitempty"`
	Note      string         `gorm:"type:text" json:"note,omitempty"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (RentFlowPlatformInvoice) TableName() string {
	return "rentflow_platform_invoices"
}
