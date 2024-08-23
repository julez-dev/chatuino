package mainui

import (
	"bytes"
	"io"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/julez-dev/chatuino/mocks"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/mock"
)

func TestUISingleChannelAnonymous(t *testing.T) {
	t.SkipNow()

	provider := mocks.NewAccountProvider(t)
	provider.EXPECT().GetAllAccounts().Return([]save.Account{
		{
			ID:          "anonymous-account",
			IsMain:      false,
			IsAnonymous: true,
			DisplayName: "justinfan123123",
			AccessToken: "oauth:123123123",
			CreatedAt:   time.Now(),
		},
	}, nil)

	emoteStore := mocks.NewEmoteStore(t)
	emoteStore.EXPECT().RefreshLocal(mock.Anything, "test-id").Return(nil)
	emoteStore.EXPECT().RefreshGlobal(mock.Anything).Return(nil)

	serverClient := mocks.NewAPIClientWithRefresh(t)
	serverClient.EXPECT().GetUsers(mock.Anything, []string{"testchannel"}, mock.AnythingOfType("[]string")).Return(twitch.UserResponse{
		Data: []twitch.UserData{
			{
				ID: "test-id",
			},
		},
	}, nil)
	serverClient.EXPECT().GetChatSettings(mock.Anything, "test-id", "").Return(twitch.GetChatSettingsResponse{
		Data: []twitch.ChatSettingData{
			{
				BroadcasterID:    "test-id",
				SlowMode:         true,
				SlowModeWaitTime: 1,
				FollowerMode:     true,
			},
		},
	}, nil)
	serverClient.EXPECT().GetStreamInfo(mock.Anything, []string{"test-id"}).Return(twitch.GetStreamsResponse{
		Data: []twitch.StreamData{
			{
				UserID:      "test-id",
				ViewerCount: 1000,
				Title:       "Welcome to my stream!",
				GameName:    "Just Chatting",
			},
		},
	}, nil)

	ttv := mocks.NewAPIClient(t)
	chatPool := mocks.NewChatPool(t)
	recentMessages := mocks.NewRecentMessageService(t)
	eventSub := mocks.NewEventSubPool(t)

	clientID := "test-xxx"

	defaultBinds := save.BuildDefaultKeyMap()

	initialModel := NewUI(zerolog.Nop(), provider, chatPool, emoteStore, clientID, serverClient, defaultBinds, recentMessages, eventSub, nil, nil, nil, UserConfiguration{})
	initialModel.buildTTVClient = func(clientID string, opt ...twitch.APIOptionFunc) (APIClient, error) {
		return ttv, nil
	}
	initialModel.loadSaveState = func() (save.AppState, error) {
		return save.AppState{}, nil
	}

	tm := teatest.NewTestModel(t, initialModel, teatest.WithInitialTermSize(45, 45))

	tm.Send(tea.KeyMsg{Type: tea.KeyF1})
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("testchannel")})
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return bytes.Contains(bts, []byte("testchannel"))
	}, teatest.WithCheckInterval(100*time.Millisecond), teatest.WithDuration(5*time.Second))

	time.Sleep(time.Second * 4)

	tm.Send(tea.Quit())
	out, err := io.ReadAll(tm.FinalOutput(t))
	if err != nil {
		t.Error(err)
	}

	teatest.RequireEqualOutput(t, out)
}
