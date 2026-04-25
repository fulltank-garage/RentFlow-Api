package controllers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"rentflow-api/config"
	"rentflow-api/models"
	"rentflow-api/services"
)

func RentFlowPartnerListMembers(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}
	var items []models.RentFlowTenantMember
	if err := config.DB.Where("tenant_id = ?", tenant.ID).Order("created_at DESC").Find(&items).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงข้อมูลทีมได้")
		return
	}
	rentFlowSuccess(c, http.StatusOK, "ดึงข้อมูลทีมสำเร็จ", gin.H{"items": rentFlowPartnerMemberResponses(items), "total": len(items)})
}

func RentFlowPartnerCreateMember(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}
	var payload struct {
		Email       string   `json:"email"`
		Name        string   `json:"name"`
		Role        string   `json:"role"`
		Permissions []string `json:"permissions"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูลทีมไม่ถูกต้อง")
		return
	}
	role := rentFlowNormalizeMemberRole(payload.Role)
	permissionsJSON := rentFlowPartnerPermissionsJSON(payload.Permissions)
	item := models.RentFlowTenantMember{
		ID:              services.NewID("mbr"),
		TenantID:        tenant.ID,
		Email:           strings.TrimSpace(strings.ToLower(payload.Email)),
		Name:            strings.TrimSpace(payload.Name),
		Role:            role,
		PermissionsJSON: permissionsJSON,
		Status:          "active",
	}
	if item.Email == "" {
		rentFlowError(c, http.StatusBadRequest, "กรุณากรอกอีเมล")
		return
	}
	if err := config.DB.Create(&item).Error; err != nil {
		rentFlowError(c, http.StatusConflict, "ไม่สามารถเพิ่มทีมได้")
		return
	}
	rentFlowAudit(c, tenant.ID, "member.create", "member", item.ID, item.Email)
	rentFlowSuccess(c, http.StatusCreated, "เพิ่มทีมสำเร็จ", rentFlowPartnerMemberResponse(item))
}

func rentFlowPartnerMemberResponses(items []models.RentFlowTenantMember) []gin.H {
	result := make([]gin.H, 0, len(items))
	for _, item := range items {
		result = append(result, rentFlowPartnerMemberResponse(item))
	}
	return result
}

func rentFlowPartnerMemberResponse(item models.RentFlowTenantMember) gin.H {
	return gin.H{
		"id":          item.ID,
		"tenantId":    item.TenantID,
		"userId":      item.UserID,
		"email":       item.Email,
		"name":        item.Name,
		"role":        item.Role,
		"permissions": rentFlowJSONList(item.PermissionsJSON),
		"status":      item.Status,
		"createdAt":   item.CreatedAt,
		"updatedAt":   item.UpdatedAt,
	}
}

func RentFlowPartnerUpdateMember(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}
	var payload struct {
		Name        string   `json:"name"`
		Role        string   `json:"role"`
		Status      string   `json:"status"`
		Permissions []string `json:"permissions"`
	}
	_ = c.ShouldBindJSON(&payload)
	status := strings.TrimSpace(payload.Status)
	if status == "" {
		status = "active"
	}
	role := rentFlowNormalizeMemberRole(payload.Role)
	result := config.DB.Model(&models.RentFlowTenantMember{}).
		Where("tenant_id = ? AND id = ?", tenant.ID, c.Param("memberId")).
		Updates(map[string]interface{}{
			"name":             strings.TrimSpace(payload.Name),
			"role":             role,
			"permissions_json": rentFlowPartnerPermissionsJSON(payload.Permissions),
			"status":           status,
			"updated_at":       time.Now(),
		})
	if result.Error != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถอัปเดตทีมได้")
		return
	}
	if result.RowsAffected == 0 {
		rentFlowError(c, http.StatusNotFound, "ไม่พบสมาชิกทีม")
		return
	}
	rentFlowAudit(c, tenant.ID, "member.update", "member", c.Param("memberId"), role)
	rentFlowSuccess(c, http.StatusOK, "อัปเดตทีมสำเร็จ", nil)
}

func RentFlowPartnerDeleteMember(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}
	result := config.DB.Where("tenant_id = ? AND id = ?", tenant.ID, c.Param("memberId")).Delete(&models.RentFlowTenantMember{})
	if result.Error != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถลบทีมได้")
		return
	}
	if result.RowsAffected == 0 {
		rentFlowError(c, http.StatusNotFound, "ไม่พบสมาชิกทีม")
		return
	}
	rentFlowAudit(c, tenant.ID, "member.delete", "member", c.Param("memberId"), "")
	rentFlowSuccess(c, http.StatusOK, "ลบทีมสำเร็จ", nil)
}

func rentFlowPartnerPermissionsJSON(items []string) string {
	if len(items) == 0 {
		return ""
	}
	normalized := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		value := strings.TrimSpace(strings.ToLower(item))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		normalized = append(normalized, value)
	}
	if len(normalized) == 0 {
		return ""
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return ""
	}
	return string(raw)
}

func RentFlowPartnerListPromotions(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}
	var items []models.RentFlowPromotion
	if err := config.DB.Where("tenant_id = ?", tenant.ID).Order("created_at DESC").Find(&items).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงโปรโมชันได้")
		return
	}
	rentFlowSuccess(c, http.StatusOK, "ดึงโปรโมชันสำเร็จ", gin.H{"items": items, "total": len(items)})
}

func RentFlowPartnerCreatePromotion(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}
	item, ok := rentFlowPromotionFromPayload(c, tenant.ID, "")
	if !ok {
		return
	}
	if err := config.DB.Create(&item).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถเพิ่มโปรโมชันได้")
		return
	}
	rentFlowAudit(c, tenant.ID, "promotion.create", "promotion", item.ID, item.Code)
	rentFlowSuccess(c, http.StatusCreated, "เพิ่มโปรโมชันสำเร็จ", item)
}

func RentFlowPartnerUpdatePromotion(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}
	item, ok := rentFlowPromotionFromPayload(c, tenant.ID, c.Param("promotionId"))
	if !ok {
		return
	}
	result := config.DB.Model(&models.RentFlowPromotion{}).
		Where("tenant_id = ? AND id = ?", tenant.ID, item.ID).
		Updates(map[string]interface{}{
			"code": item.Code, "name": item.Name, "description": item.Description,
			"discount_type": item.DiscountType, "discount_value": item.DiscountValue,
			"starts_at": item.StartsAt, "ends_at": item.EndsAt, "is_active": item.IsActive,
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถอัปเดตโปรโมชันได้")
		return
	}
	if result.RowsAffected == 0 {
		rentFlowError(c, http.StatusNotFound, "ไม่พบโปรโมชัน")
		return
	}
	rentFlowAudit(c, tenant.ID, "promotion.update", "promotion", item.ID, item.Code)
	rentFlowSuccess(c, http.StatusOK, "อัปเดตโปรโมชันสำเร็จ", item)
}

func RentFlowPartnerDeletePromotion(c *gin.Context) {
	rentFlowPartnerDeleteModel(c, "promotionId", &models.RentFlowPromotion{}, "promotion", "ลบโปรโมชันสำเร็จ")
}

func RentFlowPartnerListAddons(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}
	var items []models.RentFlowAddon
	if err := config.DB.Where("tenant_id = ?", tenant.ID).Order("created_at DESC").Find(&items).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงบริการเสริมได้")
		return
	}
	rentFlowSuccess(c, http.StatusOK, "ดึงบริการเสริมสำเร็จ", gin.H{"items": items, "total": len(items)})
}

func RentFlowPartnerCreateAddon(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}
	item, ok := rentFlowAddonFromPayload(c, tenant.ID, "")
	if !ok {
		return
	}
	if err := config.DB.Create(&item).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถเพิ่มบริการเสริมได้")
		return
	}
	rentFlowAudit(c, tenant.ID, "addon.create", "addon", item.ID, item.Name)
	rentFlowSuccess(c, http.StatusCreated, "เพิ่มบริการเสริมสำเร็จ", item)
}

func RentFlowPartnerUpdateAddon(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}
	item, ok := rentFlowAddonFromPayload(c, tenant.ID, c.Param("addonId"))
	if !ok {
		return
	}
	result := config.DB.Model(&models.RentFlowAddon{}).
		Where("tenant_id = ? AND id = ?", tenant.ID, item.ID).
		Updates(map[string]interface{}{"name": item.Name, "description": item.Description, "price": item.Price, "unit": item.Unit, "is_active": item.IsActive, "updated_at": time.Now()})
	if result.Error != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถอัปเดตบริการเสริมได้")
		return
	}
	if result.RowsAffected == 0 {
		rentFlowError(c, http.StatusNotFound, "ไม่พบบริการเสริม")
		return
	}
	rentFlowAudit(c, tenant.ID, "addon.update", "addon", item.ID, item.Name)
	rentFlowSuccess(c, http.StatusOK, "อัปเดตบริการเสริมสำเร็จ", item)
}

func RentFlowPartnerDeleteAddon(c *gin.Context) {
	rentFlowPartnerDeleteModel(c, "addonId", &models.RentFlowAddon{}, "addon", "ลบบริการเสริมสำเร็จ")
}

func RentFlowPartnerListLeads(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}
	var items []models.RentFlowLead
	if err := config.DB.Where("tenant_id = ?", tenant.ID).Order("created_at DESC").Find(&items).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถดึงลีดได้")
		return
	}
	rentFlowSuccess(c, http.StatusOK, "ดึงลีดสำเร็จ", gin.H{"items": items, "total": len(items)})
}

func RentFlowPartnerCreateLead(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}
	item, ok := rentFlowLeadFromPayload(c, tenant.ID, "")
	if !ok {
		return
	}
	if err := config.DB.Create(&item).Error; err != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถเพิ่มลีดได้")
		return
	}
	rentFlowAudit(c, tenant.ID, "lead.create", "lead", item.ID, item.Name)
	rentFlowSuccess(c, http.StatusCreated, "เพิ่มลีดสำเร็จ", item)
}

func RentFlowPartnerUpdateLead(c *gin.Context) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}
	item, ok := rentFlowLeadFromPayload(c, tenant.ID, c.Param("leadId"))
	if !ok {
		return
	}
	result := config.DB.Model(&models.RentFlowLead{}).
		Where("tenant_id = ? AND id = ?", tenant.ID, item.ID).
		Updates(map[string]interface{}{"name": item.Name, "email": item.Email, "phone": item.Phone, "source": item.Source, "status": item.Status, "interested_car": item.InterestedCar, "note": item.Note, "updated_at": time.Now()})
	if result.Error != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถอัปเดตลีดได้")
		return
	}
	if result.RowsAffected == 0 {
		rentFlowError(c, http.StatusNotFound, "ไม่พบลีด")
		return
	}
	rentFlowAudit(c, tenant.ID, "lead.update", "lead", item.ID, item.Status)
	rentFlowSuccess(c, http.StatusOK, "อัปเดตลีดสำเร็จ", item)
}

func RentFlowPartnerDeleteLead(c *gin.Context) {
	rentFlowPartnerDeleteModel(c, "leadId", &models.RentFlowLead{}, "lead", "ลบลีดสำเร็จ")
}

func rentFlowPromotionFromPayload(c *gin.Context, tenantID, id string) (models.RentFlowPromotion, bool) {
	var payload struct {
		Code          string `json:"code"`
		Name          string `json:"name"`
		Description   string `json:"description"`
		DiscountType  string `json:"discountType"`
		DiscountValue int64  `json:"discountValue"`
		StartsAt      string `json:"startsAt"`
		EndsAt        string `json:"endsAt"`
		IsActive      *bool  `json:"isActive"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูลโปรโมชันไม่ถูกต้อง")
		return models.RentFlowPromotion{}, false
	}
	if id == "" {
		id = services.NewID("prm")
	}
	code := strings.ToUpper(strings.TrimSpace(payload.Code))
	name := strings.TrimSpace(payload.Name)
	if code == "" || name == "" {
		rentFlowError(c, http.StatusBadRequest, "กรุณากรอกโค้ดและชื่อโปรโมชัน")
		return models.RentFlowPromotion{}, false
	}
	discountType := strings.TrimSpace(payload.DiscountType)
	if discountType != "amount" {
		discountType = "percent"
	}
	isActive := true
	if payload.IsActive != nil {
		isActive = *payload.IsActive
	}
	return models.RentFlowPromotion{ID: id, TenantID: tenantID, Code: code, Name: name, Description: strings.TrimSpace(payload.Description), DiscountType: discountType, DiscountValue: payload.DiscountValue, IsActive: isActive}, true
}

func rentFlowAddonFromPayload(c *gin.Context, tenantID, id string) (models.RentFlowAddon, bool) {
	var payload struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Price       int64  `json:"price"`
		Unit        string `json:"unit"`
		IsActive    *bool  `json:"isActive"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูลบริการเสริมไม่ถูกต้อง")
		return models.RentFlowAddon{}, false
	}
	if id == "" {
		id = services.NewID("add")
	}
	name := strings.TrimSpace(payload.Name)
	if name == "" {
		rentFlowError(c, http.StatusBadRequest, "กรุณากรอกชื่อบริการเสริม")
		return models.RentFlowAddon{}, false
	}
	unit := strings.TrimSpace(payload.Unit)
	if unit == "" {
		unit = "day"
	}
	isActive := true
	if payload.IsActive != nil {
		isActive = *payload.IsActive
	}
	return models.RentFlowAddon{ID: id, TenantID: tenantID, Name: name, Description: strings.TrimSpace(payload.Description), Price: payload.Price, Unit: unit, IsActive: isActive}, true
}

func rentFlowLeadFromPayload(c *gin.Context, tenantID, id string) (models.RentFlowLead, bool) {
	var payload struct {
		Name          string `json:"name"`
		Email         string `json:"email"`
		Phone         string `json:"phone"`
		Source        string `json:"source"`
		Status        string `json:"status"`
		InterestedCar string `json:"interestedCar"`
		Note          string `json:"note"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		rentFlowError(c, http.StatusBadRequest, "ข้อมูลลีดไม่ถูกต้อง")
		return models.RentFlowLead{}, false
	}
	if id == "" {
		id = services.NewID("led")
	}
	name := strings.TrimSpace(payload.Name)
	if name == "" {
		rentFlowError(c, http.StatusBadRequest, "กรุณากรอกชื่อลีด")
		return models.RentFlowLead{}, false
	}
	status := strings.TrimSpace(payload.Status)
	if status == "" {
		status = "new"
	}
	return models.RentFlowLead{ID: id, TenantID: tenantID, Name: name, Email: strings.TrimSpace(strings.ToLower(payload.Email)), Phone: strings.TrimSpace(payload.Phone), Source: strings.TrimSpace(payload.Source), Status: status, InterestedCar: strings.TrimSpace(payload.InterestedCar), Note: strings.TrimSpace(payload.Note)}, true
}

func rentFlowPartnerDeleteModel(c *gin.Context, param string, model interface{}, entity, successMessage string) {
	tenant, ok := rentFlowRequireOwnerTenant(c)
	if !ok {
		return
	}
	id := c.Param(param)
	result := config.DB.Where("tenant_id = ? AND id = ?", tenant.ID, id).Delete(model)
	if result.Error != nil {
		rentFlowError(c, http.StatusInternalServerError, "ไม่สามารถลบข้อมูลได้")
		return
	}
	if result.RowsAffected == 0 {
		rentFlowError(c, http.StatusNotFound, "ไม่พบข้อมูลที่ต้องการลบ")
		return
	}
	rentFlowAudit(c, tenant.ID, entity+".delete", entity, id, "")
	rentFlowSuccess(c, http.StatusOK, successMessage, nil)
}

func rentFlowNormalizeMemberRole(role string) string {
	switch strings.TrimSpace(strings.ToLower(role)) {
	case "owner", "finance", "staff":
		return strings.TrimSpace(strings.ToLower(role))
	default:
		return "staff"
	}
}
