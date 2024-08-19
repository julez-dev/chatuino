// Code generated by mockery v2.44.2. DO NOT EDIT.

package mocks

import (
	sql "database/sql"

	mock "github.com/stretchr/testify/mock"
)

// DB is an autogenerated mock type for the DB type
type DB struct {
	mock.Mock
}

type DB_Expecter struct {
	mock *mock.Mock
}

func (_m *DB) EXPECT() *DB_Expecter {
	return &DB_Expecter{mock: &_m.Mock}
}

// Exec provides a mock function with given fields: query, args
func (_m *DB) Exec(query string, args ...interface{}) (sql.Result, error) {
	var _ca []interface{}
	_ca = append(_ca, query)
	_ca = append(_ca, args...)
	ret := _m.Called(_ca...)

	if len(ret) == 0 {
		panic("no return value specified for Exec")
	}

	var r0 sql.Result
	var r1 error
	if rf, ok := ret.Get(0).(func(string, ...interface{}) (sql.Result, error)); ok {
		return rf(query, args...)
	}
	if rf, ok := ret.Get(0).(func(string, ...interface{}) sql.Result); ok {
		r0 = rf(query, args...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(sql.Result)
		}
	}

	if rf, ok := ret.Get(1).(func(string, ...interface{}) error); ok {
		r1 = rf(query, args...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// DB_Exec_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Exec'
type DB_Exec_Call struct {
	*mock.Call
}

// Exec is a helper method to define mock.On call
//   - query string
//   - args ...interface{}
func (_e *DB_Expecter) Exec(query interface{}, args ...interface{}) *DB_Exec_Call {
	return &DB_Exec_Call{Call: _e.mock.On("Exec",
		append([]interface{}{query}, args...)...)}
}

func (_c *DB_Exec_Call) Run(run func(query string, args ...interface{})) *DB_Exec_Call {
	_c.Call.Run(func(args mock.Arguments) {
		variadicArgs := make([]interface{}, len(args)-1)
		for i, a := range args[1:] {
			if a != nil {
				variadicArgs[i] = a.(interface{})
			}
		}
		run(args[0].(string), variadicArgs...)
	})
	return _c
}

func (_c *DB_Exec_Call) Return(_a0 sql.Result, _a1 error) *DB_Exec_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *DB_Exec_Call) RunAndReturn(run func(string, ...interface{}) (sql.Result, error)) *DB_Exec_Call {
	_c.Call.Return(run)
	return _c
}

// Query provides a mock function with given fields: query, args
func (_m *DB) Query(query string, args ...interface{}) (*sql.Rows, error) {
	var _ca []interface{}
	_ca = append(_ca, query)
	_ca = append(_ca, args...)
	ret := _m.Called(_ca...)

	if len(ret) == 0 {
		panic("no return value specified for Query")
	}

	var r0 *sql.Rows
	var r1 error
	if rf, ok := ret.Get(0).(func(string, ...interface{}) (*sql.Rows, error)); ok {
		return rf(query, args...)
	}
	if rf, ok := ret.Get(0).(func(string, ...interface{}) *sql.Rows); ok {
		r0 = rf(query, args...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*sql.Rows)
		}
	}

	if rf, ok := ret.Get(1).(func(string, ...interface{}) error); ok {
		r1 = rf(query, args...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// DB_Query_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Query'
type DB_Query_Call struct {
	*mock.Call
}

// Query is a helper method to define mock.On call
//   - query string
//   - args ...interface{}
func (_e *DB_Expecter) Query(query interface{}, args ...interface{}) *DB_Query_Call {
	return &DB_Query_Call{Call: _e.mock.On("Query",
		append([]interface{}{query}, args...)...)}
}

func (_c *DB_Query_Call) Run(run func(query string, args ...interface{})) *DB_Query_Call {
	_c.Call.Run(func(args mock.Arguments) {
		variadicArgs := make([]interface{}, len(args)-1)
		for i, a := range args[1:] {
			if a != nil {
				variadicArgs[i] = a.(interface{})
			}
		}
		run(args[0].(string), variadicArgs...)
	})
	return _c
}

func (_c *DB_Query_Call) Return(_a0 *sql.Rows, _a1 error) *DB_Query_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *DB_Query_Call) RunAndReturn(run func(string, ...interface{}) (*sql.Rows, error)) *DB_Query_Call {
	_c.Call.Return(run)
	return _c
}

// NewDB creates a new instance of DB. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewDB(t interface {
	mock.TestingT
	Cleanup(func())
}) *DB {
	mock := &DB{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
