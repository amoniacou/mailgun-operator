/*
Copyright 2024 Amoniac OU.

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

package domain

import (
	"context"

	"github.com/amoniacou/mailgun-operator/internal/configuration"
	"github.com/mailgun/mailgun-go/v4"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	domainv1 "github.com/amoniacou/mailgun-operator/api/domain/v1"
	corev1 "k8s.io/api/core/v1"
)

const (
	routeFinalizer = "route.finalizers.mailgun.com"
)

// RouteReconciler reconciles a Route object
type RouteReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Config   *configuration.Data
}

// +kubebuilder:rbac:groups=domain.mailgun.com,resources=routes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=domain.mailgun.com,resources=routes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=domain.mailgun.com,resources=routes/finalizers,verbs=update

// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *RouteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	var mailgunRoute = &domainv1.Route{}

	// lookup for item
	if err := r.Get(ctx, req.NamespacedName, mailgunRoute); err != nil {
		if client.IgnoreNotFound(err) != nil {
			log.Error(err, "unable to fetch Route")
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.V(1).Info("Start to reconcile route", "route", mailgunRoute)
	mg := r.Config.MailgunClient("")

	// examine DeletionTimestamp to determine if object is under deletion
	if !mailgunRoute.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object. This is equivalent
		// to registering our finalizer.
		if !controllerutil.ContainsFinalizer(mailgunRoute, routeFinalizer) {
			controllerutil.AddFinalizer(mailgunRoute, routeFinalizer)
			if err := r.Update(ctx, mailgunRoute); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	} else {
		// The object is being deleted
		if controllerutil.ContainsFinalizer(mailgunRoute, domainFinalizer) {
			_, err := mg.GetRoute(ctx, mailgunRoute.Status.RouteID)
			if err == nil {
				log.V(1).Info("route not exits on mailgun", "id", mailgunRoute.Status.RouteID)
				return ctrl.Result{}, nil
			}
			if err := mg.DeleteRoute(ctx, mailgunRoute.Status.RouteID); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	}

	routeRequest := mailgun.Route{
		Description: mailgunRoute.Spec.Description,
		Expression:  mailgunRoute.Spec.Expression,
		Actions:     mailgunRoute.Spec.Actions,
	}

	if mailgunRoute.Spec.Priority != nil && *mailgunRoute.Spec.Priority != 0 {
		routeRequest.Priority = *mailgunRoute.Spec.Priority
	}

	if len(mailgunRoute.Status.RouteID) > 0 {
		_, err := mg.GetRoute(ctx, mailgunRoute.Status.RouteID)
		if err == nil {
			log.V(1).Info("route not exits not mailgun", "id", mailgunRoute.Status.RouteID)
			return ctrl.Result{}, nil
		}
		_, err = mg.UpdateRoute(ctx, mailgunRoute.Status.RouteID, routeRequest)
		if err != nil {
			log.Error(err, "unable to update route")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	} else {
		routeResp, err := mg.CreateRoute(ctx, routeRequest)

		if err != nil {
			log.Error(err, "unable to create route")
			r.Recorder.Eventf(mailgunRoute, corev1.EventTypeWarning, "FailedCreate", err.Error())
			return ctrl.Result{}, err
		}
		mailgunRoute.Status.RouteID = routeResp.Id
	}

	return ctrl.Result{}, r.Status().Update(ctx, mailgunRoute)
}

func (r *RouteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	pred := predicate.GenerationChangedPredicate{}
	return ctrl.NewControllerManagedBy(mgr).
		For(&domainv1.Route{}).
		WithEventFilter(pred).
		Complete(r)
}
