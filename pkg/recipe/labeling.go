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

package recipe

import (
	"fmt"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	types "mayadata.io/d-operators/types/recipe"
	"openebs.io/metac/dynamic/clientset"
)

// Labeling helps applying desired labels(s) against the resource
type Labeling struct {
	BaseRunner
	Label *types.Label

	result *types.LabelResult
	err    error

	totalFoundCount int
	labeledCount    int
	unLabeledCount  int
}

// LabelingConfig helps in creating new instance of Labeling
type LabelingConfig struct {
	BaseRunner
	Label *types.Label
}

// NewLabeler returns a new instance of Labeling
func NewLabeler(config LabelingConfig) *Labeling {
	return &Labeling{
		BaseRunner: config.BaseRunner,
		Label:      config.Label,
		result:     &types.LabelResult{},
	}
}

// NewDefaultLabeler returns a new instance of Labeling
func NewDefaultLabeler(name string, lbl *types.Label) (*Labeling, error) {
	runner, err := NewDefaultBaseRunner(name)
	if err != nil {
		return nil, err
	}
	return &Labeling{
		BaseRunner: *runner,
		Label:      lbl,
		result:     &types.LabelResult{},
	}, nil
}

func (l *Labeling) verboseMessage() string {
	return fmt.Sprintf(
		"Resource labeling: Found %d: Labeled %d: UnLabeled %d",
		l.totalFoundCount,
		l.labeledCount,
		l.unLabeledCount,
	)
}

func (l *Labeling) unset(
	client *clientset.ResourceClient,
	obj *unstructured.Unstructured,
) error {
	var currentLbls = obj.GetLabels()
	if len(currentLbls) == 0 ||
		len(currentLbls) < len(l.Label.ApplyLabels) {
		// given object is not eligible to be
		// unset, since all the desired labels
		// are not present
		return nil
	}
	for key, val := range l.Label.ApplyLabels {
		if currentLbls[key] != val {
			// given object is not eligible to be
			// unset, since it does not match the desired labels
			return nil
		}
	}
	var newLbls = map[string]string{}
	var isUnset bool
	for key, val := range currentLbls {
		isUnset = false
		for applyKey := range l.Label.ApplyLabels {
			if key == applyKey {
				// do not add this key & value
				// In other words unset this label
				isUnset = true
				break
			}
		}
		if !isUnset {
			// add existing key value pair since
			// this is not to be unset
			newLbls[key] = val
		}
	}
	// update the resource by removing desired labels
	obj.SetLabels(newLbls)
	// update the object against the cluster
	_, err := client.
		Namespace(obj.GetNamespace()).
		Update(
			obj,
			metav1.UpdateOptions{},
		)
	if err == nil {
		l.unLabeledCount++
	}
	return err
}

func (l *Labeling) label(
	client *clientset.ResourceClient,
	obj *unstructured.Unstructured,
) error {
	var newLbls = map[string]string{}
	// fill with existing labels
	for key, val := range obj.GetLabels() {
		newLbls[key] = val
	}
	// add / update desired labels to existing labels
	for nkey, nval := range l.Label.ApplyLabels {
		newLbls[nkey] = nval
	}
	// update the resource with new labels
	obj.SetLabels(newLbls)
	// update the object against the cluster
	_, err := client.
		Namespace(obj.GetNamespace()).
		Update(
			obj,
			metav1.UpdateOptions{},
		)
	if err == nil {
		l.labeledCount++
	}
	return err
}

func (l *Labeling) labelOrUnset(
	client *clientset.ResourceClient,
	obj *unstructured.Unstructured,
) error {
	var isInclude bool
	if len(l.Label.IncludeByNames) == 0 {
		// If resource names are not provided then all
		// resources will be labeled
		isInclude = true
	}
	for _, name := range l.Label.IncludeByNames {
		if name == obj.GetName() {
			isInclude = true
			break
		}
	}
	if isInclude {
		return l.label(client, obj)
	}
	if !isInclude && l.Label.AutoUnset {
		return l.unset(client, obj)
	}
	return nil
}

func (l *Labeling) labelAll() (*types.LabelResult, error) {
	var message = fmt.Sprintf(
		"Label resource %s %s: GVK %s",
		l.Label.State.GetNamespace(),
		l.Label.State.GetName(),
		l.Label.State.GroupVersionKind(),
	)
	err := l.Retry.Waitf(
		func() (bool, error) {
			// get appropriate dynamic client
			client, err := l.GetClientForAPIVersionAndKind(
				l.Label.State.GetAPIVersion(),
				l.Label.State.GetKind(),
			)
			if err != nil {
				return false, errors.Wrapf(
					err,
					"Failed to get resource client",
				)
			}
			// list all resources
			items, err := client.
				Namespace(l.Label.State.GetNamespace()).
				List(metav1.ListOptions{
					LabelSelector: labels.Set(
						l.Label.State.GetLabels(),
					).String(),
				})
			if err != nil {
				return false, errors.Wrapf(
					err,
					"Failed to list resources",
				)
			}
			l.totalFoundCount = len(items.Items)
			for _, obj := range items.Items {
				err := l.labelOrUnset(client, &obj)
				if err != nil {
					return false, err
				}
			}
			return true, nil
		},
		message,
	)
	if err != nil {
		return nil, err
	}
	return &types.LabelResult{
		Phase:   types.LabelStatusPassed,
		Message: message,
		Verbose: l.verboseMessage(),
	}, nil
}

// Run applyies the desired labels or unsets them
// against the resource(s)
func (l *Labeling) Run() (*types.LabelResult, error) {
	if len(l.Label.ApplyLabels) == 0 {
		return nil, errors.Errorf(
			"Invalid label operation: Missing ApplyLabels",
		)
	}
	return l.labelAll()
}
