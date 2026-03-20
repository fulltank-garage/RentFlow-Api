package models

type Role struct {
	ID   uint64 `gorm:"primaryKey" json:"id"`
	Name string `gorm:"size:50;not null" json:"name"`
}