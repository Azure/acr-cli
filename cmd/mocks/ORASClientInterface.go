// Code generated by mockery v2.27.1. DO NOT EDIT.

package mocks

import (
	context "context"

	mock "github.com/stretchr/testify/mock"
)

// ORASClientInterface is an autogenerated mock type for the ORASClientInterface type
type ORASClientInterface struct {
	mock.Mock
}

// Annotate provides a mock function with given fields: ctx, repoName, reference, artifactType, annotations
func (_m *ORASClientInterface) Annotate(ctx context.Context, repoName string, reference string, artifactType string, annotations map[string]string) error {
	ret := _m.Called(ctx, repoName, reference, artifactType, annotations)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, string, string, map[string]string) error); ok {
		r0 = rf(ctx, repoName, reference, artifactType, annotations)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

type mockConstructorTestingTNewORASClientInterface interface {
	mock.TestingT
	Cleanup(func())
}

// NewORASClientInterface creates a new instance of ORASClientInterface. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewORASClientInterface(t mockConstructorTestingTNewORASClientInterface) *ORASClientInterface {
	mock := &ORASClientInterface{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}