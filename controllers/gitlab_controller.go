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
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
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
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
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
	var err error
	defer func() {
		if err != nil {
			// TODO Clean Env
		}
	}()

	gitlab := &devopsv1.Gitlab{}
	if err = r.Get(context.TODO(), req.NamespacedName, gitlab); err != nil {
		if !apierrors.IsNotFound(err) {
			glog.Error(err)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{Requeue: true}, nil
	}

	if err = createGitlabDm(r, gitlab); err != nil {
		glog.Error(err)
		return ctrl.Result{}, err
	}
	r.Recorder.Event(gitlab, corev1.EventTypeNormal, "Init", "Deploy Gitlab Complate")

	if err = createGitlabSvc(r, gitlab); err != nil {
		glog.Error(err)
		return ctrl.Result{}, err
	}

	gitlab.Status.BuildStage = "Init"
	if err := r.Status().Update(context.TODO(), gitlab); err != nil {
		glog.Error(err)
		return ctrl.Result{}, err
	}
	r.Recorder.Event(gitlab, corev1.EventTypeNormal, "Init", "Deploy Gitlab Service Complate")

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GitlabReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&devopsv1.Gitlab{}).
		Complete(r)
}

func createGitlabSvc(r *GitlabReconciler, gitlab *devopsv1.Gitlab) error {
	for _, export := range gitlab.Spec.Port {
		service := &corev1.Service{}
		service.Name = gitlab.Name + "-" + export.Name
		service.Namespace = gitlab.Namespace
		service.Spec.Selector = map[string]string{"app": gitlab.Name}
		if string(corev1.ServiceTypeClusterIP) == export.ExportType {
			service.Spec.Type = corev1.ServiceTypeClusterIP
			svcPort := corev1.ServicePort{
				Name:       export.Name,
				Port:       export.ContainerPort,
				TargetPort: intstr.FromInt(int(export.ExportPort)),
			}
			svcPorts := []corev1.ServicePort{}
			svcPorts = append(svcPorts, svcPort)
			service.Spec.Ports = svcPorts

		} else if string(corev1.ServiceTypeNodePort) == export.ExportType {
			service.Spec.Type = corev1.ServiceTypeNodePort
			svcPort := corev1.ServicePort{
				Name:       export.Name,
				Port:       export.ContainerPort,
				TargetPort: intstr.FromInt(int(export.ContainerPort)),
				NodePort:   export.ExportPort,
			}
			svcPorts := []corev1.ServicePort{}
			svcPorts = append(svcPorts, svcPort)
			service.Spec.Ports = svcPorts
		} else {
			service.Spec.Type = corev1.ServiceTypeClusterIP
			svcPort := corev1.ServicePort{
				Name:       export.Name,
				Port:       export.ContainerPort,
				TargetPort: intstr.FromInt(int(export.ExportPort)),
			}
			svcPorts := []corev1.ServicePort{}
			svcPorts = append(svcPorts, svcPort)
			service.Spec.Ports = svcPorts
		}

		if err := controllerutil.SetControllerReference(gitlab, service, r.Scheme); err != nil {
			glog.Error(err)
			return err
		}

		glog.Infof("Create Gitlab Service Success[%s/%s].", service.Namespace, service.Name)
		if err := r.Create(context.TODO(), service); err != nil {
			glog.Error(err)
			return err
		}
	}

	return nil
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
					"app": gitlab.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": gitlab.Name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            gitlab.Name,
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
			HostPort:      port.ExportPort,
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
