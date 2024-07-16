// Code generated by mockery v2.43.2. DO NOT EDIT.

package mocks

import (
	context "context"

	mock "github.com/stretchr/testify/mock"

	twitch "github.com/julez-dev/chatuino/twitch"
)

// APIClientWithRefresh is an autogenerated mock type for the APIClientWithRefresh type
type APIClientWithRefresh struct {
	mock.Mock
}

type APIClientWithRefresh_Expecter struct {
	mock *mock.Mock
}

func (_m *APIClientWithRefresh) EXPECT() *APIClientWithRefresh_Expecter {
	return &APIClientWithRefresh_Expecter{mock: &_m.Mock}
}

// GetChatSettings provides a mock function with given fields: ctx, broadcasterID, moderatorID
func (_m *APIClientWithRefresh) GetChatSettings(ctx context.Context, broadcasterID string, moderatorID string) (twitch.GetChatSettingsResponse, error) {
	ret := _m.Called(ctx, broadcasterID, moderatorID)

	if len(ret) == 0 {
		panic("no return value specified for GetChatSettings")
	}

	var r0 twitch.GetChatSettingsResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string, string) (twitch.GetChatSettingsResponse, error)); ok {
		return rf(ctx, broadcasterID, moderatorID)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string, string) twitch.GetChatSettingsResponse); ok {
		r0 = rf(ctx, broadcasterID, moderatorID)
	} else {
		r0 = ret.Get(0).(twitch.GetChatSettingsResponse)
	}

	if rf, ok := ret.Get(1).(func(context.Context, string, string) error); ok {
		r1 = rf(ctx, broadcasterID, moderatorID)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// APIClientWithRefresh_GetChatSettings_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetChatSettings'
type APIClientWithRefresh_GetChatSettings_Call struct {
	*mock.Call
}

// GetChatSettings is a helper method to define mock.On call
//   - ctx context.Context
//   - broadcasterID string
//   - moderatorID string
func (_e *APIClientWithRefresh_Expecter) GetChatSettings(ctx interface{}, broadcasterID interface{}, moderatorID interface{}) *APIClientWithRefresh_GetChatSettings_Call {
	return &APIClientWithRefresh_GetChatSettings_Call{Call: _e.mock.On("GetChatSettings", ctx, broadcasterID, moderatorID)}
}

func (_c *APIClientWithRefresh_GetChatSettings_Call) Run(run func(ctx context.Context, broadcasterID string, moderatorID string)) *APIClientWithRefresh_GetChatSettings_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(string), args[2].(string))
	})
	return _c
}

func (_c *APIClientWithRefresh_GetChatSettings_Call) Return(_a0 twitch.GetChatSettingsResponse, _a1 error) *APIClientWithRefresh_GetChatSettings_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *APIClientWithRefresh_GetChatSettings_Call) RunAndReturn(run func(context.Context, string, string) (twitch.GetChatSettingsResponse, error)) *APIClientWithRefresh_GetChatSettings_Call {
	_c.Call.Return(run)
	return _c
}

// GetStreamInfo provides a mock function with given fields: ctx, broadcastID
func (_m *APIClientWithRefresh) GetStreamInfo(ctx context.Context, broadcastID []string) (twitch.GetStreamsResponse, error) {
	ret := _m.Called(ctx, broadcastID)

	if len(ret) == 0 {
		panic("no return value specified for GetStreamInfo")
	}

	var r0 twitch.GetStreamsResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, []string) (twitch.GetStreamsResponse, error)); ok {
		return rf(ctx, broadcastID)
	}
	if rf, ok := ret.Get(0).(func(context.Context, []string) twitch.GetStreamsResponse); ok {
		r0 = rf(ctx, broadcastID)
	} else {
		r0 = ret.Get(0).(twitch.GetStreamsResponse)
	}

	if rf, ok := ret.Get(1).(func(context.Context, []string) error); ok {
		r1 = rf(ctx, broadcastID)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// APIClientWithRefresh_GetStreamInfo_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetStreamInfo'
type APIClientWithRefresh_GetStreamInfo_Call struct {
	*mock.Call
}

// GetStreamInfo is a helper method to define mock.On call
//   - ctx context.Context
//   - broadcastID []string
func (_e *APIClientWithRefresh_Expecter) GetStreamInfo(ctx interface{}, broadcastID interface{}) *APIClientWithRefresh_GetStreamInfo_Call {
	return &APIClientWithRefresh_GetStreamInfo_Call{Call: _e.mock.On("GetStreamInfo", ctx, broadcastID)}
}

func (_c *APIClientWithRefresh_GetStreamInfo_Call) Run(run func(ctx context.Context, broadcastID []string)) *APIClientWithRefresh_GetStreamInfo_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].([]string))
	})
	return _c
}

func (_c *APIClientWithRefresh_GetStreamInfo_Call) Return(_a0 twitch.GetStreamsResponse, _a1 error) *APIClientWithRefresh_GetStreamInfo_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *APIClientWithRefresh_GetStreamInfo_Call) RunAndReturn(run func(context.Context, []string) (twitch.GetStreamsResponse, error)) *APIClientWithRefresh_GetStreamInfo_Call {
	_c.Call.Return(run)
	return _c
}

// GetUsers provides a mock function with given fields: ctx, logins, ids
func (_m *APIClientWithRefresh) GetUsers(ctx context.Context, logins []string, ids []string) (twitch.UserResponse, error) {
	ret := _m.Called(ctx, logins, ids)

	if len(ret) == 0 {
		panic("no return value specified for GetUsers")
	}

	var r0 twitch.UserResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, []string, []string) (twitch.UserResponse, error)); ok {
		return rf(ctx, logins, ids)
	}
	if rf, ok := ret.Get(0).(func(context.Context, []string, []string) twitch.UserResponse); ok {
		r0 = rf(ctx, logins, ids)
	} else {
		r0 = ret.Get(0).(twitch.UserResponse)
	}

	if rf, ok := ret.Get(1).(func(context.Context, []string, []string) error); ok {
		r1 = rf(ctx, logins, ids)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// APIClientWithRefresh_GetUsers_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetUsers'
type APIClientWithRefresh_GetUsers_Call struct {
	*mock.Call
}

// GetUsers is a helper method to define mock.On call
//   - ctx context.Context
//   - logins []string
//   - ids []string
func (_e *APIClientWithRefresh_Expecter) GetUsers(ctx interface{}, logins interface{}, ids interface{}) *APIClientWithRefresh_GetUsers_Call {
	return &APIClientWithRefresh_GetUsers_Call{Call: _e.mock.On("GetUsers", ctx, logins, ids)}
}

func (_c *APIClientWithRefresh_GetUsers_Call) Run(run func(ctx context.Context, logins []string, ids []string)) *APIClientWithRefresh_GetUsers_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].([]string), args[2].([]string))
	})
	return _c
}

func (_c *APIClientWithRefresh_GetUsers_Call) Return(_a0 twitch.UserResponse, _a1 error) *APIClientWithRefresh_GetUsers_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *APIClientWithRefresh_GetUsers_Call) RunAndReturn(run func(context.Context, []string, []string) (twitch.UserResponse, error)) *APIClientWithRefresh_GetUsers_Call {
	_c.Call.Return(run)
	return _c
}

// RefreshToken provides a mock function with given fields: ctx, refreshToken
func (_m *APIClientWithRefresh) RefreshToken(ctx context.Context, refreshToken string) (string, string, error) {
	ret := _m.Called(ctx, refreshToken)

	if len(ret) == 0 {
		panic("no return value specified for RefreshToken")
	}

	var r0 string
	var r1 string
	var r2 error
	if rf, ok := ret.Get(0).(func(context.Context, string) (string, string, error)); ok {
		return rf(ctx, refreshToken)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string) string); ok {
		r0 = rf(ctx, refreshToken)
	} else {
		r0 = ret.Get(0).(string)
	}

	if rf, ok := ret.Get(1).(func(context.Context, string) string); ok {
		r1 = rf(ctx, refreshToken)
	} else {
		r1 = ret.Get(1).(string)
	}

	if rf, ok := ret.Get(2).(func(context.Context, string) error); ok {
		r2 = rf(ctx, refreshToken)
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// APIClientWithRefresh_RefreshToken_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'RefreshToken'
type APIClientWithRefresh_RefreshToken_Call struct {
	*mock.Call
}

// RefreshToken is a helper method to define mock.On call
//   - ctx context.Context
//   - refreshToken string
func (_e *APIClientWithRefresh_Expecter) RefreshToken(ctx interface{}, refreshToken interface{}) *APIClientWithRefresh_RefreshToken_Call {
	return &APIClientWithRefresh_RefreshToken_Call{Call: _e.mock.On("RefreshToken", ctx, refreshToken)}
}

func (_c *APIClientWithRefresh_RefreshToken_Call) Run(run func(ctx context.Context, refreshToken string)) *APIClientWithRefresh_RefreshToken_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(string))
	})
	return _c
}

func (_c *APIClientWithRefresh_RefreshToken_Call) Return(_a0 string, _a1 string, _a2 error) *APIClientWithRefresh_RefreshToken_Call {
	_c.Call.Return(_a0, _a1, _a2)
	return _c
}

func (_c *APIClientWithRefresh_RefreshToken_Call) RunAndReturn(run func(context.Context, string) (string, string, error)) *APIClientWithRefresh_RefreshToken_Call {
	_c.Call.Return(run)
	return _c
}

// NewAPIClientWithRefresh creates a new instance of APIClientWithRefresh. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewAPIClientWithRefresh(t interface {
	mock.TestingT
	Cleanup(func())
}) *APIClientWithRefresh {
	mock := &APIClientWithRefresh{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}