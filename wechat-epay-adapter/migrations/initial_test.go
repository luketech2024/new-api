package migrations

import (
	"testing"

	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/store"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestApplyInitialCreatesAllPaymentTables(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, ApplyInitial(db))
	require.NoError(t, ApplyAlipayOrderColumns(db))

	migrator := db.Migrator()
	assert.True(t, migrator.HasTable(&store.PaymentOrder{}))
	assert.True(t, migrator.HasTable(&store.NotificationTask{}))
	assert.True(t, migrator.HasTable(&store.PaymentAuditEvent{}))
	assert.True(t, migrator.HasColumn(&store.PaymentOrder{}, "AlipayQrCode"))
	assert.True(t, migrator.HasColumn(&store.PaymentOrder{}, "AlipayTradeNo"))
	assert.True(t, migrator.HasColumn(&store.PaymentOrder{}, "AlipayNotifyID"))
}
