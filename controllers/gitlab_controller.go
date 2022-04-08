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
	"fmt"

	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	gitlabdm := &appsv1.Deployment{}
	if err = r.Get(context.TODO(), req.NamespacedName, gitlabdm); err != nil {
		if apierrors.IsNotFound(err) {
			r.Recorder.Event(gitlab, corev1.EventTypeNormal, "Init", "Can Not Find Deployment, Maybe In The Construction Stage.")
		}
	} else {
		if gitlabdm.Status.Replicas > 0 && gitlabdm.Status.Replicas == gitlabdm.Status.ReadyReplicas && gitlab.Status.BuildStage != "Ready" {
			gitlab.Status.BuildStage = "Ready"
			if err := r.Status().Update(context.TODO(), gitlab); err != nil {
				glog.Error(err)
				return ctrl.Result{}, err
			}
			r.Recorder.Event(gitlab, corev1.EventTypeNormal, "Init", "Init Deployment Success.")
			return ctrl.Result{}, nil
		} else if gitlabdm.Status.Replicas > 0 && gitlabdm.Status.Replicas != gitlabdm.Status.ReadyReplicas && gitlab.Status.BuildStage != "NotReady" {
			gitlab.Status.BuildStage = "NotReady"
			if err := r.Status().Update(context.TODO(), gitlab); err != nil {
				glog.Error(err)
				return ctrl.Result{}, err
			}
			r.Recorder.Event(gitlab, corev1.EventTypeNormal, "Init", "Wait Deployment Ready...")
			return ctrl.Result{}, nil
		}
	}

	if len(gitlabdm.Spec.Template.Labels) != 0 {
		gitlabsvc := &corev1.ServiceList{}
		if err = r.List(context.TODO(), gitlabsvc, client.MatchingLabels(gitlabdm.Spec.Template.Labels)); err != nil {
			if apierrors.IsNotFound(err) {
				gitlab.Status.Network = "Pre Init"
				if err := r.Status().Update(context.TODO(), gitlab); err != nil {
					glog.Error(err)
					return ctrl.Result{}, err
				}
			}
		} else {
			waitnetwork := false
			for _, svc := range gitlabsvc.Items {
				endpoint := &corev1.Endpoints{}
				nsname := types.NamespacedName{
					Name:      svc.Name,
					Namespace: svc.Namespace,
				}
				if err = r.Get(context.TODO(), nsname, endpoint); err != nil {
					if apierrors.IsNotFound(err) {
						msg := fmt.Sprintf("Wait For Endpoint Initialization Corresponding To The Service [%s] To Complete.", svc.Name)
						r.Recorder.Event(gitlab, corev1.EventTypeNormal, "Init", msg)
						return ctrl.Result{}, nil
					}
				} else {
					if len(endpoint.Subsets) == 0 {
						break
					}
					for _, sub := range endpoint.Subsets {
						if len(sub.NotReadyAddresses) != 0 {
							waitnetwork = true
						}
					}
					if !waitnetwork && gitlab.Status.Network != "Ready" {
						gitlab.Status.Network = "Ready"
						if err := r.Status().Update(context.TODO(), gitlab); err != nil {
							glog.Error(err)
							return ctrl.Result{}, err
						}
						r.Recorder.Event(gitlab, corev1.EventTypeNormal, "Init", "Init Network Success.")
						return ctrl.Result{}, nil
					}
				}
			}
		}
	}

	if err = r.reconcilerGitlabDm(gitlab, req); err != nil {
		glog.Error(err)
		return ctrl.Result{}, err
	}

	if err = r.reconcilerGitlabSvc(gitlab, req); err != nil {
		glog.Error(err)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GitlabReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&devopsv1.Gitlab{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}

func (r *GitlabReconciler) reconcilerGitlabDm(gitlab *devopsv1.Gitlab, req ctrl.Request) error {
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
					NodeSelector: map[string]string{
						"kubernetes.io/hostname": gitlab.Spec.NodeSelector,
					},
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
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "volumes-data",
									MountPath: "/etc/gitlab",
									SubPath:   "config",
								},
								{
									Name:      "volumes-data",
									MountPath: "/var/log/gitlab",
									SubPath:   "logs",
								},
								{
									Name:      "volumes-data",
									MountPath: "/var/opt/gitlab",
									SubPath:   "data",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "volumes-data",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: gitlab.Spec.VolumeName,
								},
							},
						},
					},
				},
			},
		},
	}
	oldDm := &appsv1.Deployment{}
	if err := r.Get(context.TODO(), req.NamespacedName, oldDm); err != nil && apierrors.IsNotFound(err) {
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
		rp := &corev1.Probe{
			InitialDelaySeconds: 180,
			PeriodSeconds:       5,
			FailureThreshold:    30,
		}
		rp.Exec = &corev1.ExecAction{
			Command: []string{
				"/bin/bash", "-c", "/opt/gitlab/bin/gitlab-healthcheck",
			},
		}
		gitlabDm.Spec.Template.Spec.Containers[0].ReadinessProbe = rp

		if err := controllerutil.SetControllerReference(gitlab, gitlabDm, r.Scheme); err != nil {
			glog.Error(err)
			return err
		}

		glog.Infof("Create Gitlab Deployment Success[%s/%s].", gitlab.Namespace, gitlab.Name)
		r.Create(context.TODO(), gitlabDm)
	} else {
		// todo update
	}

	return nil
}

func (r *GitlabReconciler) reconcilerGitlabSvc(gitlab *devopsv1.Gitlab, req ctrl.Request) error {
	for _, export := range gitlab.Spec.Port {
		service := &corev1.Service{}
		service.Name = gitlab.Name + "-" + export.Name
		service.Labels = map[string]string{
			"app": gitlab.Name,
		}
		service.Namespace = gitlab.Namespace
		oldSvc := &corev1.Service{}
		svcNameNs := types.NamespacedName{
			Name:      service.Name,
			Namespace: req.Namespace,
		}
		if err := r.Get(context.TODO(), svcNameNs, oldSvc); err != nil && apierrors.IsNotFound(err) {
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
		} else {
			//todo update
		}
	}

	return nil
}
