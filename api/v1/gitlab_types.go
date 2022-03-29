/*
Copyright 2022.

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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type ContainerPort struct {
	// Export Port Name
	Name string `json:"name,omitempty"`
	// Host Port
	HostPort int32 `json:"hostport,omitempty"`
	// Container Port
	ContainerPort int32 `json:"containerport"`
}

// GitlabSpec defines the desired state of Gitlab
type GitlabSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Gitlab Container Image
	Image string `json:"image"`
	// Gitlab Export Port List
	Port []ContainerPort `json:"port,omitempty"`
	// Gitlab Admin Account Default Password,Also Use Env GITLAB_ROOT_PASSWORD To Specify Default Passwd
	DefaultPassword string `json:"defaultpassword,omitempty"`
}

// GitlabStatus defines the observed state of Gitlab
type GitlabStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Health string `json:"health,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Gitlab is the Schema for the gitlabs API
type Gitlab struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GitlabSpec   `json:"spec,omitempty"`
	Status GitlabStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// GitlabList contains a list of Gitlab
type GitlabList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Gitlab `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Gitlab{}, &GitlabList{})
}