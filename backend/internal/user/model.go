package user

import "time"

// User 表示系统用户。
// 第一版可以先使用固定 user_id，后续接 JWT 后再完善注册和登录。
type User struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	Email     string    `gorm:"size:128;uniqueIndex;not null" json:"email"`
	Nickname  string    `gorm:"size:64;not null" json:"nickname"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 固定表名，避免 Gorm 复数规则变化影响迁移。
func (User) TableName() string {
	return "users"
}
