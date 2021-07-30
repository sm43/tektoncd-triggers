/*
Copyright 2021 The Tekton Authors

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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/tektoncd/triggers/pkg/apis/triggers/v1alpha1"
	v1beta12 "github.com/tektoncd/triggers/pkg/apis/triggers/v1beta1"
	triggersclientset "github.com/tektoncd/triggers/pkg/client/clientset/versioned"
	"github.com/tektoncd/triggers/pkg/client/injection/reconciler/triggers/v1alpha1/syncrepo"
	"github.com/tektoncd/triggers/pkg/resources"
	"github.com/tektoncd/triggers/pkg/template"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	discoveryclient "k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"knative.dev/pkg/logging"
	pkgreconciler "knative.dev/pkg/reconciler"
)

var (
	// Check that our Reconciler implements syncrepo.Interface
	_ syncrepo.Interface = (*Reconciler)(nil)
	// Check that our Reconciler implements syncrepo.Finalizer
	_ syncrepo.Finalizer = (*Reconciler)(nil)
)

// Reconciler implements controller.Reconciler for Configuration resources.
type Reconciler struct {
	DiscoveryClient   discoveryclient.ServerResourcesInterface
	DynamicClientSet  dynamic.Interface
	KubeClientSet     kubernetes.Interface
	TriggersClientSet triggersclientset.Interface
	Requeue           func(obj interface{}, after time.Duration)
}

func (r *Reconciler) ReconcileKind(ctx context.Context, sr *v1alpha1.SyncRepo) pkgreconciler.Event {

	logger := logging.FromContext(ctx)

	fmt.Println("************************************************************")
	fmt.Println("What's the time :P ", time.Now())
	fmt.Println("************************************************************")

	res, err := r.TriggersClientSet.TriggersV1alpha1().SyncRepos(sr.Namespace).Get(ctx, sr.Name, metav1.GetOptions{})
	if err != nil {
		logger.Error("Error occurred while fetching sync repo: ", err)
		return err
	}

	logger.Info("Repo: ", res.Spec.Repo)

	tb, err := r.TriggersClientSet.TriggersV1beta1().TriggerBindings(sr.Namespace).Get(ctx, sr.Spec.TriggerBinding, metav1.GetOptions{})
	if err != nil {
		logger.Error("Error occurred while getting tb: ", err)
		return err
	}

	logger.Info("TB: ", tb.Name)

	tt, err := r.TriggersClientSet.TriggersV1beta1().TriggerTemplates(sr.Namespace).Get(ctx, sr.Spec.TriggerTemplate, metav1.GetOptions{})
	if err != nil {
		logger.Error("Error occurred while getting tb: ", err)
		return err
	}

	logger.Info("TT: ", tt.Name)

	syncDur, err := getSyncDuration(sr)
	if err != nil {
		logger.Info("got error while converting frequency to duration ", syncDur)
		return err
	}

	logger.Info("Sync freq is: ", syncDur)

	updatedRepo := strings.ReplaceAll(res.Spec.Repo, "https://", "")
	logger.Info("replace repo: ", updatedRepo)

	repoArr := strings.Split(updatedRepo, "/")
	logger.Info("split array is: ", repoArr)

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/commits/%s", repoArr[1], repoArr[2], res.Spec.Branch)
	logger.Info("url is: ", url)

	// Do all the validations before this

	data, code, err := httpGet(url)
	if err != nil {
		logger.Error("Error occurred while fetching repo details: ", err)
		return err
	}

	// There is rate limit for github api
	if code == http.StatusForbidden {
		logger.Info("^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^")
		logger.Error("I am forbidden.. lets come back later")
		logger.Info("^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^")
		r.Requeue(sr, time.Minute*30)
		return nil
	}

	if code != http.StatusOK {
		logger.Error("invalid status code ", code)
		// set status and return ?
		return nil
	}

	logger.Info("Status code: ", code)

	var gc map[string]interface{}
	err = json.Unmarshal(data, &gc)
	if err != nil {
		logger.Error("Error occurred while json unmarshalling: ", err)
		// requeue immediately or after a duration?
		return nil
	}

	sha := gc["sha"].(string)
	logger.Info("Sha is: ", sha)

	if sr.Status.LastCommit != "" && sr.Status.LastCommit == sha {
		logger.Info("commit is not changed to requeue", sha)
		logger.Info("re queuing with sync duration: ", syncDur)
		r.Requeue(sr, syncDur)
		return nil
	}

	p, err := template.ApplyEventValuesToParams(tb.Spec.Params, data, http.Header{}, map[string]interface{}{}, []v1beta12.ParamSpec{})
	if err != nil {
		logger.Error("templating failed ", err)
		return nil
	}

	logger.Infof("ResolvedParams : %+v", p)
	resolvedRes := template.ResolveResources(tt, p)

	// Error Handling should be better
	// the req should be requeue immediately only in cases when there is chance of getting it
	// executed successfully in next reconciler
	// if we keep re queueing on each error the we might hit
	// github api rate limit and further http call will be forbidden

	for _, re := range resolvedRes {
		data := new(unstructured.Unstructured)
		if err := data.UnmarshalJSON(re); err != nil {
			logger.Errorf("couldn't unmarshal json from the TriggerTemplate: %v", err)
			return nil
		}

		logger.Info("trigger template data : ", data)

		apiResource, err := resources.FindAPIResource(data.GetAPIVersion(), data.GetKind(), r.DiscoveryClient)
		if err != nil {
			logger.Error("Error occurred while api resource: ", err)
			return nil
		}

		name := data.GetName()
		if name == "" {
			name = data.GetGenerateName()
		}
		logger.Infof("Generating resource: kind: %s, name: %s", apiResource, name)

		gvr := schema.GroupVersionResource{
			Group:    apiResource.Group,
			Version:  apiResource.Version,
			Resource: apiResource.Name,
		}

		if _, err := r.DynamicClientSet.Resource(gvr).Namespace("default").Create(context.Background(), data, metav1.CreateOptions{}); err != nil {
			if kerrors.IsUnauthorized(err) || kerrors.IsForbidden(err) {
				logger.Errorf("couldn't create resource with group version kind %q: %v", gvr, err)
				return nil
			}
			logger.Errorf("couldn't create resource with group version kind %q: %v", gvr, err)
			return nil
		}
	}

	sr.Status.LastCommit = sha
	_, err = r.TriggersClientSet.TriggersV1alpha1().SyncRepos(sr.Namespace).UpdateStatus(ctx, sr, metav1.UpdateOptions{})
	if err != nil {
		logger.Error("error while updating status for syncrepo ", err)
		return nil
	}

	r.Requeue(sr, syncDur)
	return nil
}

// FinalizeKind cleans up associated logging config maps when an EventListener is deleted
func (r *Reconciler) FinalizeKind(ctx context.Context, sr *v1alpha1.SyncRepo) pkgreconciler.Event {
	return nil
}

// httpGet gets raw data given the url
func httpGet(url string) ([]byte, int, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}

	return data, resp.StatusCode, err
}

func getSyncDuration(repo *v1alpha1.SyncRepo) (time.Duration, error) {
	freq, err := time.ParseDuration(repo.Spec.Frequency)
	if err != nil {
		return 0, err
	}
	return freq, nil
}
