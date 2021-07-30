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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genreconciler:krshapedlogic=false
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SyncRepo takes parameters and uses them to sync a remote repo
// to trigger a pipeline
// +k8s:openapi-gen=true
type SyncRepo struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Spec holds the desired state of the SyncRepo from the client
	// +optional
	Spec SyncRepoSpec `json:"spec"`
	// +optional
	Status SyncRepoStatus `json:"status,omitempty"`
}

type SyncRepoSpec struct {
	Repo            string `json:"repo,omitempty"`
	Branch          string `json:"branch,omitempty"`
	Frequency       string `json:"frequency,omitempty"`
	TriggerBinding  string `json:"binding,omitempty"`
	TriggerTemplate string `json:"template,omitempty"`
}

type SyncRepoStatus struct {
	LastCommit string `json:"lastCommit,omitempty"`
}

// SyncRepoList contains a list of SyncRepo
//
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type SyncRepoList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SyncRepo `json:"items"`
}
