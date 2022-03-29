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

package controllers

import (
	"context"

	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	devopsv1 "gitlab-operator/api/v1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// GitlabReconciler reconciles a Gitlab object
type GitlabReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=devops.gitlab.domain,resources=gitlabs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=devops.gitlab.domain,resources=gitlabs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=devops.gitlab.domain,resources=gitlabs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Gitlab object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *GitlabReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	gitlab := &devopsv1.Gitlab{}
	if err := r.Get(context.TODO(), req.NamespacedName, gitlab); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{Requeue: true}, err
	}

	if err := createGitlabDm(r, gitlab); err != nil {
		return ctrl.Result{Requeue: true}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GitlabReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&devopsv1.Gitlab{}).
		Complete(r)
}

func createGitlabDm(r *GitlabReconciler, gitlab *devopsv1.Gitlab) error {
	gitlabDm := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gitlab.Name,
			Namespace: gitlab.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointer.Int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": gitlab.Namespace,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": gitlab.Namespace,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            gitlab.Namespace,
							Image:           gitlab.Spec.Image,
							ImagePullPolicy: "IfNotPresent",
							Env: []corev1.EnvVar{
								{
									Name:  "GITLAB_ROOT_PASSWORD",
									Value: gitlab.Spec.DefaultPassword,
								},
							},
						},
					},
				},
			},
		},
	}

	dmports := []corev1.ContainerPort{}
	for _, port := range gitlab.Spec.Port {
		dmport := corev1.ContainerPort{
			Name:          port.Name,
			HostPort:      port.HostPort,
			ContainerPort: port.ContainerPort,
		}
		dmports = append(dmports, dmport)
	}

	gitlabDm.Spec.Template.Spec.Containers[0].Ports = dmports

	if err := controllerutil.SetControllerReference(gitlab, gitlabDm, r.Scheme); err != nil {
		glog.Error(err)
		return err
	}

	glog.Infof("Create Gitlab Deployment Success[%s/%s].", gitlabDm.Namespace, gitlab.Name)
	r.Create(context.TODO(), gitlabDm)

	return nil
}
