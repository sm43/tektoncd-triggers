/*
Copyright 2019 The Tekton Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package syncrepo

import (
	"context"
	"log"

	triggersclient "github.com/tektoncd/triggers/pkg/client/injection/client"
	syncrepoinformer "github.com/tektoncd/triggers/pkg/client/injection/informers/triggers/v1alpha1/syncrepo"
	syncreporeconciler "github.com/tektoncd/triggers/pkg/client/injection/reconciler/triggers/v1alpha1/syncrepo"
	"github.com/tektoncd/triggers/pkg/sink"
	"k8s.io/client-go/rest"
	kubeclient "knative.dev/pkg/client/injection/kube/client"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/injection/clients/dynamicclient"
	"knative.dev/pkg/logging"
)

// NewController creates a new instance of an EventListener controller.
func NewController() func(context.Context, configmap.Watcher) *controller.Impl {
	return func(ctx context.Context, cmw configmap.Watcher) *controller.Impl {
		logger := logging.FromContext(ctx)

		clusterConfig, err := rest.InClusterConfig()
		if err != nil {
			log.Fatalf("Failed to get in cluster config: %v", err)
		}

		dynamicClientSet := dynamicclient.Get(ctx)
		kubeClientSet := kubeclient.Get(ctx)
		triggersClientSet := triggersclient.Get(ctx)
		syncRepoInformer := syncrepoinformer.Get(ctx)

		sinkClients, err := sink.ConfigureClients(clusterConfig)
		if err != nil {
			logger.Fatal(err)
		}

		r := &Reconciler{
			DiscoveryClient:   sinkClients.DiscoveryClient,
			DynamicClientSet:  dynamicClientSet,
			KubeClientSet:     kubeClientSet,
			TriggersClientSet: triggersClientSet,
		}

		impl := syncreporeconciler.NewImpl(ctx, r)

		// Pass enqueue func to reconciler
		r.Requeue = impl.EnqueueAfter

		logger.Info("Setting up event handlers")

		syncRepoInformer.Informer().AddEventHandler(controller.HandleAll(impl.Enqueue))

		return impl
	}
}
