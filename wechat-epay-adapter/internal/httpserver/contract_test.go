package httpserver

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouteContractsFreezePublicSurface(t *testing.T) {
	require.Len(t, RouteContracts, 10)
	assert.Contains(t, RouteContracts, RouteContract{Method: http.MethodPost, Path: RouteSubmit, Authentication: "epay_md5"})
	assert.Contains(t, RouteContracts, RouteContract{Method: http.MethodPost, Path: RouteWechatNotification, Authentication: "wechat_public_key"})
	assert.Contains(t, RouteContracts, RouteContract{Method: http.MethodPost, Path: RouteAlipayNotification, Authentication: "alipay_rsa2"})
	assert.Contains(t, RouteContracts, RouteContract{Method: http.MethodPost, Path: RouteAdminRetry, Authentication: "admin_bearer"})
	assert.Equal(t, "/api/v1/alipay/notify", RouteAlipayNotification)
}

func TestStatusForErrorMatchesContract(t *testing.T) {
	tests := map[ErrorCode]int{
		ErrorInvalidRequest:  http.StatusBadRequest,
		ErrorForbidden:       http.StatusForbidden,
		ErrorOrderConflict:   http.StatusConflict,
		ErrorNotFound:        http.StatusNotFound,
		ErrorNotReady:        http.StatusServiceUnavailable,
		ErrorUpstreamFailure: http.StatusBadGateway,
		ErrorTemporary:       http.StatusInternalServerError,
	}
	for code, expected := range tests {
		assert.Equal(t, expected, StatusForError(code), string(code))
	}
}

func TestWechatNotificationStatusMatchesContract(t *testing.T) {
	assert.Equal(t, http.StatusNoContent, StatusForWechatNotification(WechatNotificationPersisted))
	assert.Equal(t, http.StatusOK, StatusForWechatNotification(WechatNotificationUnknownOrder))
	assert.Equal(t, http.StatusBadRequest, StatusForWechatNotification(WechatNotificationInvalid))
	assert.Equal(t, http.StatusInternalServerError, StatusForWechatNotification(WechatNotificationTemporary))
	assert.Equal(t, http.StatusAccepted, StatusNotificationRetryAccepted)
}

func TestResponseForAlipayNotification_UnknownOrder_ReturnsSuccessBody(t *testing.T) {
	status, body := ResponseForAlipayNotification(AlipayNotificationUnknownOrder)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "success", body)
	assert.NotEqual(t, http.StatusNoContent, status)
}

func TestResponseForAlipayNotification_Persisted_ReturnsSuccessBody(t *testing.T) {
	status, body := ResponseForAlipayNotification(AlipayNotificationPersisted)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "success", body)
}

func TestResponseForAlipayNotification_InvalidOrTemporary_DoesNotAckSuccess(t *testing.T) {
	invalidStatus, invalidBody := ResponseForAlipayNotification(AlipayNotificationInvalid)
	assert.Equal(t, http.StatusBadRequest, invalidStatus)
	assert.NotEqual(t, "success", invalidBody)

	tempStatus, tempBody := ResponseForAlipayNotification(AlipayNotificationTemporary)
	assert.Equal(t, http.StatusInternalServerError, tempStatus)
	assert.NotEqual(t, "success", tempBody)
}

func TestCashierStatusResponseIncludesPaymentType(t *testing.T) {
	payload, err := json.Marshal(CashierStatusResponse{PaymentType: "alipay", Status: "PAYABLE"})
	require.NoError(t, err)
	assert.Contains(t, string(payload), `"payment_type":"alipay"`)
	assert.NotContains(t, string(payload), `"wechat_trade_no"`)
	assert.NotContains(t, string(payload), `"alipay_trade_no"`)
}
