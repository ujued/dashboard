// Copyright 2017 The Kubernetes Dashboard Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package replicaset

import (
	"errors"
	"reflect"
	"testing"

	"github.com/kubernetes/dashboard/src/app/backend/api"
	"github.com/kubernetes/dashboard/src/app/backend/resource/common"
	"github.com/kubernetes/dashboard/src/app/backend/resource/dataselect"
	"github.com/kubernetes/dashboard/src/app/backend/resource/metric"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/pkg/api/v1"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

func getReplicasPointer(replicas int32) *int32 {
	return &replicas
}

func TestGetReplicaSetListFromChannels(t *testing.T) {
	controller := true
	cases := []struct {
		k8sRs         extensions.ReplicaSetList
		k8sRsError    error
		pods          *v1.PodList
		expected      *ReplicaSetList
		expectedError error
	}{
		{
			extensions.ReplicaSetList{},
			nil,
			&v1.PodList{},
			&ReplicaSetList{
				ListMeta:          api.ListMeta{},
				CumulativeMetrics: make([]metric.Metric, 0),
				ReplicaSets:       []ReplicaSet{}},
			nil,
		},
		{
			extensions.ReplicaSetList{},
			errors.New("MyCustomError"),
			&v1.PodList{},
			nil,
			errors.New("MyCustomError"),
		},
		{
			extensions.ReplicaSetList{},
			&k8serrors.StatusError{},
			&v1.PodList{},
			nil,
			&k8serrors.StatusError{},
		},
		{
			extensions.ReplicaSetList{},
			&k8serrors.StatusError{ErrStatus: metaV1.Status{}},
			&v1.PodList{},
			nil,
			&k8serrors.StatusError{ErrStatus: metaV1.Status{}},
		},
		{
			extensions.ReplicaSetList{},
			&k8serrors.StatusError{ErrStatus: metaV1.Status{Reason: "foo-bar"}},
			&v1.PodList{},
			nil,
			&k8serrors.StatusError{ErrStatus: metaV1.Status{Reason: "foo-bar"}},
		},
		{
			extensions.ReplicaSetList{},
			&k8serrors.StatusError{ErrStatus: metaV1.Status{Reason: "NotFound"}},
			&v1.PodList{},
			&ReplicaSetList{
				ReplicaSets: make([]ReplicaSet, 0),
			},
			nil,
		},
		{
			extensions.ReplicaSetList{
				Items: []extensions.ReplicaSet{{
					ObjectMeta: metaV1.ObjectMeta{
						Name:              "rs-name",
						Namespace:         "rs-namespace",
						Labels:            map[string]string{"key": "value"},
						UID:               "uid",
						CreationTimestamp: metaV1.Unix(111, 222),
					},
					Spec: extensions.ReplicaSetSpec{
						Selector: &metaV1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
						Replicas: getReplicasPointer(21),
					},
					Status: extensions.ReplicaSetStatus{
						Replicas: 7,
					},
				}},
			},
			nil,
			&v1.PodList{
				Items: []v1.Pod{
					{
						ObjectMeta: metaV1.ObjectMeta{
							Namespace: "rs-namespace",
							OwnerReferences: []metaV1.OwnerReference{
								{
									Name:       "rs-name",
									UID:        "uid",
									Controller: &controller,
								},
							},
						},
						Status: v1.PodStatus{Phase: v1.PodFailed},
					},
					{
						ObjectMeta: metaV1.ObjectMeta{
							Namespace: "rs-namespace",
							OwnerReferences: []metaV1.OwnerReference{
								{
									Name:       "rs-name-wrong",
									UID:        "uid-wrong",
									Controller: &controller,
								},
							},
						},
						Status: v1.PodStatus{Phase: v1.PodFailed},
					},
				},
			},
			&ReplicaSetList{
				ListMeta:          api.ListMeta{TotalItems: 1},
				CumulativeMetrics: make([]metric.Metric, 0),
				ReplicaSets: []ReplicaSet{{
					ObjectMeta: api.ObjectMeta{
						Name:              "rs-name",
						Namespace:         "rs-namespace",
						Labels:            map[string]string{"key": "value"},
						CreationTimestamp: metaV1.Unix(111, 222),
					},
					TypeMeta: api.TypeMeta{Kind: api.ResourceKindReplicaSet},
					Pods: common.PodInfo{
						Current:  7,
						Desired:  21,
						Failed:   1,
						Warnings: []common.Event{},
					},
				}},
			},
			nil,
		},
	}

	for _, c := range cases {
		channels := &common.ResourceChannels{
			ReplicaSetList: common.ReplicaSetListChannel{
				List:  make(chan *extensions.ReplicaSetList, 1),
				Error: make(chan error, 1),
			},
			NodeList: common.NodeListChannel{
				List:  make(chan *v1.NodeList, 1),
				Error: make(chan error, 1),
			},
			ServiceList: common.ServiceListChannel{
				List:  make(chan *v1.ServiceList, 1),
				Error: make(chan error, 1),
			},
			PodList: common.PodListChannel{
				List:  make(chan *v1.PodList, 1),
				Error: make(chan error, 1),
			},
			EventList: common.EventListChannel{
				List:  make(chan *v1.EventList, 1),
				Error: make(chan error, 1),
			},
		}

		channels.ReplicaSetList.Error <- c.k8sRsError
		channels.ReplicaSetList.List <- &c.k8sRs

		channels.NodeList.List <- &v1.NodeList{}
		channels.NodeList.Error <- nil

		channels.ServiceList.List <- &v1.ServiceList{}
		channels.ServiceList.Error <- nil

		channels.PodList.List <- c.pods
		channels.PodList.Error <- nil

		channels.EventList.List <- &v1.EventList{}
		channels.EventList.Error <- nil

		actual, err := GetReplicaSetListFromChannels(channels, dataselect.NoDataSelect, nil)
		if !reflect.DeepEqual(actual, c.expected) {
			t.Errorf("GetReplicaSetListChannels() ==\n          %#v\nExpected: %#v", actual, c.expected)
		}
		if !reflect.DeepEqual(err, c.expectedError) {
			t.Errorf("GetReplicaSetListChannels() ==\n          %#v\nExpected: %#v", err, c.expectedError)
		}
	}
}

func TestCreateReplicaSetList(t *testing.T) {
	replicas := int32(0)
	cases := []struct {
		replicaSets []extensions.ReplicaSet
		pods        []v1.Pod
		events      []v1.Event
		expected    *ReplicaSetList
	}{
		{
			[]extensions.ReplicaSet{
				{
					ObjectMeta: metaV1.ObjectMeta{Name: "replica-set", Namespace: "ns-1"},
					Spec: extensions.ReplicaSetSpec{
						Replicas: &replicas,
						Selector: &metaV1.LabelSelector{
							MatchLabels: map[string]string{"key": "value"},
						}},
				},
			},
			[]v1.Pod{},
			[]v1.Event{},
			&ReplicaSetList{
				ListMeta:          api.ListMeta{TotalItems: 1},
				CumulativeMetrics: make([]metric.Metric, 0),
				ReplicaSets: []ReplicaSet{
					{
						ObjectMeta: api.ObjectMeta{Name: "replica-set", Namespace: "ns-1"},
						TypeMeta:   api.TypeMeta{Kind: api.ResourceKindReplicaSet},
						Pods:       common.PodInfo{Warnings: []common.Event{}},
					},
				},
			},
		},
	}

	for _, c := range cases {
		actual := CreateReplicaSetList(c.replicaSets, c.pods, c.events, dataselect.NoDataSelect, nil)

		if !reflect.DeepEqual(actual, c.expected) {
			t.Errorf("CreateReplicaSetList(%#v, %#v, %#v, ...) == \ngot %#v, \nexpected %#v",
				c.replicaSets, c.pods, c.events, actual, c.expected)
		}
	}
}
