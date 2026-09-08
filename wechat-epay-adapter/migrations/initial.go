package migrations

import (
	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/store"
	"gorm.io/gorm"
)

// ApplyInitial creates the payment tables and portable indexes on a fresh database.
func ApplyInitial(db *gorm.DB) error {
	if err := store.Migrate(db); err != nil {
		return err
	}
	return ApplyAlipayOrderColumns(db)
}
