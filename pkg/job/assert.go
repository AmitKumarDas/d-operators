/*
Copyright 2020 The MayaData Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package job

import (
	"github.com/pkg/errors"
	types "mayadata.io/d-operators/types/job"
)

// Assertable is used to perform matches of desired state(s)
// against observed state(s)
type Assertable struct {
	*Fixture
	Retry  *Retryable
	Name   string
	Assert *types.Assert

	assertCheckType types.AssertCheckType
	retryOnDiff     bool
	retryOnEqual    bool
	status          *types.AssertStatus

	// error as value
	err error
}

// AssertableConfig is used to create an instance of Assertable
type AssertableConfig struct {
	Fixture *Fixture
	Retry   *Retryable
	Name    string
	Assert  *types.Assert
}

// NewAsserter returns a new instance of Assertion
func NewAsserter(config AssertableConfig) *Assertable {
	return &Assertable{
		Assert:  config.Assert,
		Retry:   config.Retry,
		Fixture: config.Fixture,
		Name:    config.Name,
		status:  &types.AssertStatus{},
	}
}

func (a *Assertable) init() {
	var checks int
	if a.Assert.PathCheck != nil {
		checks++
		a.assertCheckType = types.AssertCheckTypePath
	}
	if a.Assert.StateCheck != nil {
		checks++
		a.assertCheckType = types.AssertCheckTypeState
	}
	if checks > 1 {
		a.err = errors.Errorf(
			"Failed to assert %q: More than one assert checks found",
			a.Name,
		)
		return
	}
	if checks == 0 {
		// assert default to StateCheck based assertion
		a.Assert.StateCheck = &types.StateCheck{
			Operator: types.StateCheckOperatorEquals,
		}
	}
}

func (a *Assertable) runAssertByPath() {
	chk := NewPathChecker(
		PathCheckingConfig{
			Name:      a.Name,
			Fixture:   a.Fixture,
			State:     a.Assert.State,
			PathCheck: *a.Assert.PathCheck,
			Retry:     a.Retry,
		},
	)
	got, err := chk.Run()
	if err != nil {
		a.err = err
		return
	}
	a.status = &types.AssertStatus{
		Phase:   got.Phase.ToAssertResultPhase(),
		Message: got.Message,
		Verbose: got.Verbose,
		Warning: got.Warning,
	}
}

func (a *Assertable) runAssertByState() {
	chk := NewStateChecker(
		StateCheckingConfig{
			Name:       a.Name,
			Fixture:    a.Fixture,
			State:      a.Assert.State,
			StateCheck: *a.Assert.StateCheck,
			Retry:      a.Retry,
		},
	)
	got, err := chk.Run()
	if err != nil {
		a.err = err
		return
	}
	a.status = &types.AssertStatus{
		Phase:   got.Phase.ToAssertResultPhase(),
		Message: got.Message,
		Verbose: got.Verbose,
		Warning: got.Warning,
	}
}

func (a *Assertable) runAssert() {
	switch a.assertCheckType {
	case types.AssertCheckTypePath:
		a.runAssertByPath()
	case types.AssertCheckTypeState:
		a.runAssertByState()
	default:
		a.err = errors.Errorf(
			"Failed to run assert %q: Invalid operator %q",
			a.Name,
			a.assertCheckType,
		)
	}
}

// Run executes the assertion
func (a *Assertable) Run() (types.AssertStatus, error) {
	if a.Name == "" {
		return types.AssertStatus{}, errors.Errorf(
			"Failed to run assert: Missing assert name",
		)
	}
	if a.Assert == nil || a.Assert.State == nil {
		return types.AssertStatus{}, errors.Errorf(
			"Failed to run assert %q: Nil assert state",
			a.Name,
		)
	}
	var fns = []func(){
		a.init,
		a.runAssert,
	}
	for _, fn := range fns {
		fn()
		if a.err != nil {
			return types.AssertStatus{}, a.err
		}
	}
	return *a.status, nil
}