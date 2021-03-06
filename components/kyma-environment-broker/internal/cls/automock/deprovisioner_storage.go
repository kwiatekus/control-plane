// Code generated by mockery v2.6.0. DO NOT EDIT.

package automock

import (
	internal "github.com/kyma-project/control-plane/components/kyma-environment-broker/internal"
	mock "github.com/stretchr/testify/mock"
)

// DeprovisionerStorage is an autogenerated mock type for the DeprovisionerStorage type
type DeprovisionerStorage struct {
	mock.Mock
}

// FindInstance provides a mock function with given fields: globalAccountID
func (_m *DeprovisionerStorage) FindInstance(globalAccountID string) (*internal.CLSInstance, bool, error) {
	ret := _m.Called(globalAccountID)

	var r0 *internal.CLSInstance
	if rf, ok := ret.Get(0).(func(string) *internal.CLSInstance); ok {
		r0 = rf(globalAccountID)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*internal.CLSInstance)
		}
	}

	var r1 bool
	if rf, ok := ret.Get(1).(func(string) bool); ok {
		r1 = rf(globalAccountID)
	} else {
		r1 = ret.Get(1).(bool)
	}

	var r2 error
	if rf, ok := ret.Get(2).(func(string) error); ok {
		r2 = rf(globalAccountID)
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// MarkAsBeingRemoved provides a mock function with given fields: version, globalAccountID, skrInstanceID
func (_m *DeprovisionerStorage) MarkAsBeingRemoved(version int, globalAccountID string, skrInstanceID string) error {
	ret := _m.Called(version, globalAccountID, skrInstanceID)

	var r0 error
	if rf, ok := ret.Get(0).(func(int, string, string) error); ok {
		r0 = rf(version, globalAccountID, skrInstanceID)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// RemoveInstance provides a mock function with given fields: version, globalAccountID
func (_m *DeprovisionerStorage) RemoveInstance(version int, globalAccountID string) error {
	ret := _m.Called(version, globalAccountID)

	var r0 error
	if rf, ok := ret.Get(0).(func(int, string) error); ok {
		r0 = rf(version, globalAccountID)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Unreference provides a mock function with given fields: version, globalAccountID, skrInstanceID
func (_m *DeprovisionerStorage) Unreference(version int, globalAccountID string, skrInstanceID string) error {
	ret := _m.Called(version, globalAccountID, skrInstanceID)

	var r0 error
	if rf, ok := ret.Get(0).(func(int, string, string) error); ok {
		r0 = rf(version, globalAccountID, skrInstanceID)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}
