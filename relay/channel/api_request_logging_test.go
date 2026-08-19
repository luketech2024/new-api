package channel

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsSensitiveLogHeader(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"Authorization",
		"Cookie",
		"X-Api-Key",
		"X-Goog-Api-Key",
		"api-key",
		"Proxy-Authorization",
	} {
		assert.True(t, isSensitiveLogHeader(name), name)
	}

	assert.False(t, isSensitiveLogHeader("Content-Type"))
	assert.False(t, isSensitiveLogHeader("Anthropic-Version"))
}
