package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
)

func TestWebSearchChannelCapability(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{
		WebSearchOptions: &dto.WebSearchOptions{},
	}

	tests := []struct {
		name              string
		info              *relaycommon.RelayInfo
		wantUseResponses  bool
		wantRejectRequest bool
	}{
		{
			name: "native Anthropic channel uses Claude web search conversion",
			info: &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeAnthropic},
			},
		},
		{
			name: "OpenAI Responses channel enabled for web search",
			info: &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelType: constant.ChannelTypeOpenAI,
					ChannelOtherSettings: dto.ChannelOtherSettings{
						OpenAIResponsesWebSearchEnabled: true,
					},
				},
			},
			wantUseResponses: true,
		},
		{
			name: "OpenAI-compatible channel without declared search support is rejected",
			info: &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI},
			},
			wantRejectRequest: true,
		},
		{
			name: "unsupported channel type is rejected",
			info: &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeDeepSeek},
			},
			wantRejectRequest: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantUseResponses, shouldUseResponsesWebSearch(tt.info, request))
			assert.Equal(t, tt.wantRejectRequest, shouldRejectUnsupportedWebSearch(tt.info, request))
		})
	}
}
