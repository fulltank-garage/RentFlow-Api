package services

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"rentflow-api/config"
)

const (
	RentFlowSessionCookieName = "rentflow_session"
	rentFlowSessionPrefix     = "rentflow:session:"
	rentFlowCachePrefix       = "rentflow:cache:"
	defaultSessionTTL         = 7 * 24 * time.Hour
)

type RentFlowSession struct {
	UserID    string    `json:"userId"`
	UserEmail string    `json:"userEmail"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type memorySessionEntry struct {
	Session   RentFlowSession
	SessionID string
}

var (
	memorySessionMu sync.RWMutex
	memorySessions  = map[string]memorySessionEntry{}
)

func NewID(prefix string) string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(buf))
}

func NewBookingCode() string {
	return fmt.Sprintf("BK-%s", strings.ToUpper(hex.EncodeToString(randomBytes(4))))
}

func randomBytes(size int) []byte {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return []byte(fmt.Sprintf("%d", time.Now().UnixNano()))
	}
	return buf
}

func HashPasswordIfNeeded(password string) (string, error) {
	if strings.TrimSpace(password) == "" {
		return "", nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func CheckPassword(password, hash string) bool {
	if hash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func ParseDateTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("ไม่พบวันและเวลา")
	}

	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			if layout == "2006-01-02" {
				return parsed, nil
			}
			return parsed.UTC(), nil
		}
	}

	return time.Time{}, fmt.Errorf("รูปแบบวันและเวลาไม่ถูกต้อง")
}

func TotalDaysBetween(start, end time.Time) int {
	if !end.After(start) {
		return 0
	}
	hours := end.Sub(start).Hours() / 24
	days := int(math.Ceil(hours))
	if days < 1 {
		return 1
	}
	return days
}

func DiscountPercent(days int) int64 {
	switch {
	case days >= 30:
		return 20
	case days >= 14:
		return 15
	case days >= 7:
		return 10
	case days >= 3:
		return 5
	default:
		return 0
	}
}

func ComputeBookingPrice(pricePerDay int64, pickupDate, returnDate time.Time, pickupLocation, returnLocation string) (int, int64, int64, int64, int64) {
	totalDays := TotalDaysBetween(pickupDate, returnDate)
	subtotal := pricePerDay * int64(totalDays)
	discountPct := DiscountPercent(totalDays)
	discount := (subtotal * discountPct) / 100

	extraCharge := int64(0)
	if pickupLocation != "" && returnLocation != "" && !strings.EqualFold(strings.TrimSpace(pickupLocation), strings.TrimSpace(returnLocation)) {
		extraCharge = 500
	}

	total := subtotal - discount + extraCharge
	return totalDays, subtotal, extraCharge, discount, total
}

func CreateSession(ctx context.Context, session RentFlowSession, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		ttl = defaultSessionTTL
	}

	session.ExpiresAt = time.Now().Add(ttl)
	token := hex.EncodeToString(randomBytes(24))

	if config.RDB != nil {
		payload, err := json.Marshal(session)
		if err != nil {
			return "", err
		}
		if err := config.RDB.Set(ctx, rentFlowSessionPrefix+token, payload, ttl).Err(); err != nil {
			return "", err
		}
		return token, nil
	}

	memorySessionMu.Lock()
	memorySessions[token] = memorySessionEntry{Session: session, SessionID: token}
	memorySessionMu.Unlock()
	return token, nil
}

func GetSession(ctx context.Context, token string) (*RentFlowSession, error) {
	if strings.TrimSpace(token) == "" {
		return nil, nil
	}

	if config.RDB != nil {
		raw, err := config.RDB.Get(ctx, rentFlowSessionPrefix+token).Result()
		if err != nil {
			if err == redis.Nil {
				return nil, nil
			}
			return nil, err
		}

		var session RentFlowSession
		if err := json.Unmarshal([]byte(raw), &session); err != nil {
			return nil, err
		}
		if time.Now().After(session.ExpiresAt) {
			_ = DeleteSession(ctx, token)
			return nil, nil
		}
		return &session, nil
	}

	memorySessionMu.RLock()
	entry, ok := memorySessions[token]
	memorySessionMu.RUnlock()
	if !ok || time.Now().After(entry.Session.ExpiresAt) {
		if ok {
			_ = DeleteSession(ctx, token)
		}
		return nil, nil
	}
	return &entry.Session, nil
}

func DeleteSession(ctx context.Context, token string) error {
	if strings.TrimSpace(token) == "" {
		return nil
	}

	if config.RDB != nil {
		return config.RDB.Del(ctx, rentFlowSessionPrefix+token).Err()
	}

	memorySessionMu.Lock()
	delete(memorySessions, token)
	memorySessionMu.Unlock()
	return nil
}

func CacheKey(parts ...string) string {
	prefix := rentFlowCachePrefix + "default"
	keyParts := parts
	if len(parts) > 0 {
		prefix = strings.TrimSpace(parts[0])
		if !strings.HasPrefix(prefix, rentFlowCachePrefix) {
			prefix = rentFlowCachePrefix + prefix
		}
		keyParts = parts[1:]
	}

	hash := sha1.Sum([]byte(strings.Join(keyParts, "|")))
	return prefix + ":" + hex.EncodeToString(hash[:])
}

func CacheGetJSON(ctx context.Context, key string, target interface{}) bool {
	if config.RDB == nil {
		return false
	}

	raw, err := config.RDB.Get(ctx, key).Result()
	if err != nil {
		return false
	}

	if err := json.Unmarshal([]byte(raw), target); err != nil {
		return false
	}
	return true
}

func CacheSetJSON(ctx context.Context, key string, value interface{}, ttl time.Duration) {
	if config.RDB == nil {
		return
	}

	payload, err := json.Marshal(value)
	if err != nil {
		log.Printf("แปลงข้อมูลแคชไม่สำเร็จสำหรับ %s: %v", key, err)
		return
	}

	if err := config.RDB.Set(ctx, key, payload, ttl).Err(); err != nil {
		log.Printf("บันทึกแคชไม่สำเร็จสำหรับ %s: %v", key, err)
	}
}

func CacheDeleteByPrefix(ctx context.Context, prefix string) {
	if config.RDB == nil {
		return
	}

	var cursor uint64
	for {
		keys, nextCursor, err := config.RDB.Scan(ctx, cursor, prefix+"*", 50).Result()
		if err != nil {
			return
		}

		if len(keys) > 0 {
			_ = config.RDB.Del(ctx, keys...).Err()
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
}

func RentFlowCarsCachePrefix() string {
	return rentFlowCachePrefix + "cars"
}

func ExpandDateRange(start, end time.Time) []string {
	if end.Before(start) {
		return nil
	}

	var days []string
	current := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	last := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)
	for !current.After(last) {
		days = append(days, current.Format("2006-01-02"))
		current = current.AddDate(0, 0, 1)
	}
	return days
}

func UniqueSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			set[value] = struct{}{}
		}
	}

	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
