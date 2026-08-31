package delivery

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/order"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHTTPClientDisablesEnvironmentProxyAndValidatesRedirects(t *testing.T) {
	validator := func(_ context.Context, raw string) error {
		if raw == "https://api.example.com/redirect" {
			return errors.New("redirect rejected")
		}
		return nil
	}
	client := NewHTTPClient(validator)
	transport, ok := client.Transport.(*secureTransport)
	require.True(t, ok)
	assert.Nil(t, transport.base.Proxy)

	redirectURL, err := url.Parse("https://api.example.com/redirect")
	require.NoError(t, err)
	request := &http.Request{URL: redirectURL}
	assert.EqualError(t, client.CheckRedirect(request, nil), "redirect rejected")
}

func TestValidateAllowlistedDestinationGuardsEveryConfiguredCallbackPath(t *testing.T) {
	policy, err := order.NewNotifyURLPolicy([]string{walletNotifyURL, subscriptionNotifyURL})
	require.NoError(t, err)
	validator := ValidateAllowlistedDestination(policy)

	assert.NoError(t, validator(context.Background(), walletNotifyURL))
	assert.NoError(t, validator(context.Background(), subscriptionNotifyURL))
	assert.Error(t, validator(context.Background(), "https://api.example.com/api/other/epay/notify"))
	assert.Error(t, validator(context.Background(), "https://evil.example.com/api/user/epay/notify"))
	assert.Error(t, ValidateAllowlistedDestination(nil)(context.Background(), walletNotifyURL))
}

func TestSecureTransportRejectsRequestBeforeNetworkAccess(t *testing.T) {
	client := NewHTTPClient(func(context.Context, string) error { return errors.New("blocked") })
	request, err := http.NewRequest(http.MethodPost, "https://api.example.com/notify", nil)
	require.NoError(t, err)
	_, err = client.Do(request)
	assert.EqualError(t, err, "Post \"https://api.example.com/notify\": blocked")
}
