package migrations

import (
	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/store"
	"gorm.io/gorm"
)

// ApplyAlipayOrderColumns adds nullable Alipay columns and unique indexes without rewriting WeChat rows.
func ApplyAlipayOrderColumns(db *gorm.DB) error {
	return store.EnsureAlipayOrderColumns(db)
}
