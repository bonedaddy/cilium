// Copyright 2016-2020 Authors of Cilium
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

package watchers

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"

	cilium_v2 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2"
	ciliumv2 "github.com/cilium/cilium/pkg/k8s/client/clientset/versioned/typed/cilium.io/v2"
	"github.com/cilium/cilium/pkg/k8s/informer"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
)

const identityIndex = "identity"

var (
	errNoCE  = errors.New("object has is not a *cilium_v2.CiliumEndpoint meta")
	indexers = cache.Indexers{
		cache.NamespaceIndex: cache.MetaNamespaceIndexFunc,
		identityIndex:        identityIndexFunc,
	}

	CiliumEndpointStore cache.Indexer
)

// identityIndexFunc index identities by ID.
func identityIndexFunc(obj interface{}) ([]string, error) {
	switch t := obj.(type) {
	case *cilium_v2.CiliumEndpoint:
		if t.Status.Identity != nil {
			id := strconv.FormatInt(t.Status.Identity.ID, 10)
			return []string{id}, nil
		}
		return []string{"0"}, nil
	}
	return nil, fmt.Errorf("%w but a %s", errNoCE, reflect.TypeOf(obj))
}

// CiliumEndpointsInit starts a CiliumEndpointWatcher
func CiliumEndpointsInit(ciliumNPClient ciliumv2.CiliumV2Interface) {
	CiliumEndpointStore = cache.NewIndexer(cache.DeletionHandlingMetaNamespaceKeyFunc, indexers)

	ciliumEndpointInformer := informer.NewInformerWithStore(
		cache.NewListWatchFromClient(ciliumNPClient.RESTClient(),
			"ciliumendpoints", v1.NamespaceAll, fields.Everything()),
		&cilium_v2.CiliumEndpoint{},
		0,
		cache.ResourceEventHandlerFuncs{},
		convertToCiliumEndpoint,
		CiliumEndpointStore,
	)
	go ciliumEndpointInformer.Run(wait.NeverStop)
}

// convertToCiliumEndpoint converts a CiliumEndpoint to a minimal CiliumEndpoint
// containing only a minimal set of entities used to identity a CiliumEndpoint
// Not intended to use the CiliumEndpoints store in this cache to use them to
// update CiliumEndpoints stored in Kubernetes.
func convertToCiliumEndpoint(obj interface{}) interface{} {
	switch concreteObj := obj.(type) {
	case *cilium_v2.CiliumEndpoint:
		p := &cilium_v2.CiliumEndpoint{
			TypeMeta: concreteObj.TypeMeta,
			ObjectMeta: metav1.ObjectMeta{
				Name:      concreteObj.Name,
				Namespace: concreteObj.Namespace,
			},
			Status: cilium_v2.EndpointStatus{
				Identity: concreteObj.Status.Identity,
			},
		}
		*concreteObj = cilium_v2.CiliumEndpoint{}
		return p
	case cache.DeletedFinalStateUnknown:
		ciliumEndpoint, ok := concreteObj.Obj.(*cilium_v2.CiliumEndpoint)
		if !ok {
			return obj
		}
		dfsu := cache.DeletedFinalStateUnknown{
			Key: concreteObj.Key,
			Obj: &cilium_v2.CiliumEndpoint{
				TypeMeta: ciliumEndpoint.TypeMeta,
				ObjectMeta: metav1.ObjectMeta{
					Name:      ciliumEndpoint.Name,
					Namespace: ciliumEndpoint.Namespace,
				},
				Status: cilium_v2.EndpointStatus{
					Identity: ciliumEndpoint.Status.Identity,
				},
			},
		}
		*ciliumEndpoint = cilium_v2.CiliumEndpoint{}
		return dfsu
	default:
		return obj
	}
}

// HasCEWithIdentity returns true or false if the Cilium Endpoint store has
// the given identity.
func HasCEWithIdentity(identity string) bool {
	if CiliumEndpointStore == nil {
		return false
	}
	ces, _ := CiliumEndpointStore.ByIndex(identityIndex, identity)

	return len(ces) != 0
}
