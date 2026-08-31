package order

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resolverWith(addresses ...string) Resolver {
	return func(context.Context, string) ([]net.IP, error) {
		result := make([]net.IP, 0, len(addresses))
		for _, address := range addresses {
			result = append(result, net.ParseIP(address))
		}
		return result, nil
	}
}

func TestReturnURLPolicyAcceptsNormalizedAllowlistedPublicURL(t *testing.T) {
	policy, err := NewReturnURLPolicy("https://app.example.com/console/", resolverWith("8.8.8.8"))
	require.NoError(t, err)

	validated, err := policy.Validate(context.Background(), "https://APP.example.com/console/../console/billing?tab=history#ignored")

	require.NoError(t, err)
	assert.Equal(t, "https://app.example.com/console/billing?tab=history", validated.String())
}

func TestReturnURLPolicyRejectsUnsafeOrUnallowlistedTargets(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		resolver Resolver
	}{
		{name: "non HTTPS", raw: "http://app.example.com/console/", resolver: resolverWith("8.8.8.8")},
		{name: "userinfo", raw: "https://user@app.example.com/console/", resolver: resolverWith("8.8.8.8")},
		{name: "unexpected port", raw: "https://app.example.com:8443/console/", resolver: resolverWith("8.8.8.8")},
		{name: "path escape", raw: "https://app.example.com/admin", resolver: resolverWith("8.8.8.8")},
		{name: "loopback", raw: "https://app.example.com/console/", resolver: resolverWith("127.0.0.1")},
		{name: "private IPv6", raw: "https://app.example.com/console/", resolver: resolverWith("fd00::1")},
		{name: "mixed DNS response", raw: "https://app.example.com/console/", resolver: resolverWith("8.8.8.8", "169.254.1.1")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy, err := NewReturnURLPolicy("https://app.example.com/console/", test.resolver)
			require.NoError(t, err)
			_, err = policy.Validate(context.Background(), test.raw)
			assert.Error(t, err)
		})
	}
}

func TestNotifyURLPolicyAcceptsEveryConfiguredCallbackPath(t *testing.T) {
	policy, err := NewNotifyURLPolicy([]string{
		"https://api.example.com/api/user/epay/notify",
		"https://api.example.com/api/subscription/epay/notify",
	})
	require.NoError(t, err)

	tests := []struct {
		name      string
		candidate string
	}{
		{name: "wallet top-up", candidate: "https://api.example.com/api/user/epay/notify"},
		{name: "subscription purchase", candidate: "https://api.example.com/api/subscription/epay/notify"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			canonical, err := policy.Canonical(test.candidate)
			require.NoError(t, err)
			assert.Equal(t, test.candidate, canonical)
		})
	}
}

func TestNotifyURLPolicyCanonicalizesAndRejectsUnallowlistedDestinations(t *testing.T) {
	policy, err := NewNotifyURLPolicy([]string{" https://API.example.com/api/user/epay/notify ", "https://api.example.com/api/user/epay/notify"})
	require.NoError(t, err)

	canonical, err := policy.Canonical("https://API.example.com/api/user/./epay/notify")
	require.NoError(t, err)
	assert.Equal(t, "https://api.example.com/api/user/epay/notify", canonical)

	rejected := []struct {
		name      string
		candidate string
	}{
		{name: "path not allowlisted", candidate: "https://api.example.com/api/subscription/epay/notify"},
		{name: "host not allowlisted", candidate: "https://evil.example.com/api/user/epay/notify"},
		{name: "unexpected port", candidate: "https://api.example.com:8443/api/user/epay/notify"},
		{name: "non HTTPS", candidate: "http://api.example.com/api/user/epay/notify"},
		{name: "userinfo", candidate: "https://user@api.example.com/api/user/epay/notify"},
	}
	for _, test := range rejected {
		t.Run(test.name, func(t *testing.T) {
			_, err := policy.Canonical(test.candidate)
			assert.Error(t, err)
		})
	}
}

func TestNewNotifyURLPolicyRejectsEmptyOrInvalidAllowlist(t *testing.T) {
	_, err := NewNotifyURLPolicy(nil)
	assert.Error(t, err)

	_, err = NewNotifyURLPolicy([]string{"https://api.example.com/api/user/epay/notify", "not-a-url"})
	assert.Error(t, err)
}
