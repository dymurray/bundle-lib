//
// Copyright (c) 2017 Red Hat, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Red Hat trademarks are not licensed under Apache License, Version 2.
// No permission is granted to use or replicate Red Hat trademarks that
// are incorporated in this software or its documentation.
//

package apb

import (
	logging "github.com/op/go-logging"
	"github.com/openshift/ansible-service-broker/pkg/clients"
	"github.com/openshift/ansible-service-broker/pkg/metrics"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/kubernetes/pkg/api/v1"
)

// Deprovision - runs the abp with the deprovision action.
func Deprovision(
	instance *ServiceInstance, clusterConfig ClusterConfig, log *logging.Logger,
) (string, error) {
	log.Notice("============================================================")
	log.Notice("                      DEPROVISIONING                        ")
	log.Notice("============================================================")
	log.Noticef("ServiceInstance.Id: %s", instance.Spec.ID)
	log.Noticef("ServiceInstance.Name: %v", instance.Spec.FQName)
	log.Noticef("ServiceInstance.Image: %s", instance.Spec.Image)
	log.Noticef("ServiceInstance.Description: %s", instance.Spec.Description)
	log.Notice("============================================================")

	// Explicitly error out if image field is missing from instance.Spec
	// was introduced as a change to the apb instance.Spec to support integration
	// with the broker and still allow for providing an img path
	// Legacy ansibleapps will hit this.
	// TODO: Move this validation to a Spec creation function (yet to be created)
	if instance.Spec.Image == "" {
		log.Error("No image field found on the apb instance.Spec (apb.yaml)")
		log.Error("apb instance.Spec requires [name] and [image] fields to be separate")
		log.Error("Are you trying to run a legacy ansibleapp without an image field?")
		return "", errors.New("No image field found on instance.Spec")
	}

	nsDeleted, err := isNamespaceDeleted(instance.Context.Namespace, log)
	if err != nil {
		return "", err
	}

	// If the namespace is gone or terminating, assume that we don't need to deprovision
	// because everything is going to be deleted.  We may need to revisit this and perform
	// the deprovision inside a terminating namespace.
	if nsDeleted {
		return "", nil
	}

	// Might need to change up this interface to feed in instance ids
	sm := NewServiceAccountManager(log)
	metrics.ActionStarted("deprovision")
	executionContext, err := ExecuteApb(
		"deprovision", clusterConfig, instance.Spec,
		instance.Context, instance.Parameters, log,
	)
	defer sm.DestroyApbSandbox(executionContext, clusterConfig)
	if err != nil {
		log.Errorf("Problem executing apb [%s] deprovision:", executionContext.PodName)
		return executionContext.PodName, err
	}

	podOutput, err := watchPod(executionContext.PodName, executionContext.Namespace, log)
	if err != nil {
		log.Errorf("Error returned from watching pod\nerror: %s", err.Error())
		log.Errorf("output: %s", podOutput)
		return executionContext.PodName, err
	}

	return executionContext.PodName, err
}

func isNamespaceDeleted(name string, log *logging.Logger) (bool, error) {
	k8scli, err := clients.Kubernetes(log)
	if err != nil {
		return false, err
	}

	namespace, err := k8scli.CoreV1().Namespaces().Get(name, metav1.GetOptions{})
	if err != nil {
		return false, err
	}

	return namespace == nil || namespace.Status.Phase == v1.NamespaceTerminating, nil
}
