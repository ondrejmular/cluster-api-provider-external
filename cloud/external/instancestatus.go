/*
Copyright 2018 The Kubernetes Authors.

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

package external

import (
	"bytes"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/serializer/json"

	"k8s.io/apimachinery/pkg/runtime/schema"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/util"
)

// Long term, we should retrieve the current status by asking k8s, gce etc. for all the needed info.
// For now, it is stored in the matching CRD under an annotation. This is similar to
// the spec and status concept where the machine CRD is the instance spec and the annotation is the instance status.

const InstanceStatusAnnotationKey = "instance-status"

type instanceStatus *clusterv1.Machine

// Get the status of the instance identified by the given machine
func (ext *ExtClient) instanceStatus(machine *clusterv1.Machine) (instanceStatus, error) {
	if ext.v1Alpha1Client == nil {
		return nil, nil
	}
	currentMachine, err := util.GetMachineIfExists(ext.v1Alpha1Client.Machines(machine.Namespace), machine.ObjectMeta.Name)
	if strings.Contains(err.Error(), "could not find") {
		currentMachine = nil

	} else if err != nil {
		return nil, fmt.Errorf("GetMachineIfExists: %v", err)
	}

	if currentMachine == nil {
		// The current status no longer exists because the matching CRD has been deleted (or does not exist yet ie. bootstrapping)
		return nil, nil
	}
	return ext.machineInstanceStatus(currentMachine)
}

// Sets the status of the instance identified by the given machine to the given machine
func (ext *ExtClient) updateInstanceStatus(machine *clusterv1.Machine) error {
	if ext.v1Alpha1Client == nil {
		return nil
	}
	status := instanceStatus(machine)
	currentMachine, err := util.GetMachineIfExists(ext.v1Alpha1Client.Machines(machine.Namespace), machine.ObjectMeta.Name)
	if strings.Contains(err.Error(), "could not find") {
		err = nil
	} else if err != nil {
		return err
	}

	if currentMachine == nil {
		// The current status no longer exists because the matching CRD has been deleted.
		return fmt.Errorf("Machine has already been deleted. Cannot update current instance status for machine %v", machine.ObjectMeta.Name)
	}

	m, err := ext.setMachineInstanceStatus(currentMachine, status)
	if err != nil {
		return err
	}

	_, err = ext.v1Alpha1Client.Machines(machine.Namespace).Update(m)
	return err
}

// Gets the state of the instance stored on the given machine CRD
func (ext *ExtClient) machineInstanceStatus(machine *clusterv1.Machine) (instanceStatus, error) {
	if machine.ObjectMeta.Annotations == nil {
		// No state
		return nil, nil
	}

	a := machine.ObjectMeta.Annotations[InstanceStatusAnnotationKey]
	if a == "" {
		// No state
		return nil, nil
	}

	serializer := json.NewSerializer(json.DefaultMetaFactory, ext.scheme, ext.scheme, false)
	var status clusterv1.Machine
	_, _, err := serializer.Decode([]byte(a), &schema.GroupVersionKind{Group: "", Version: "cluster.k8s.io/v1alpha1", Kind: "Machine"}, &status)
	if err != nil {
		return nil, fmt.Errorf("decoding failure: %v", err)
	}

	return instanceStatus(&status), nil
}

// Applies the state of an instance onto a given machine CRD
func (ext *ExtClient) setMachineInstanceStatus(machine *clusterv1.Machine, status instanceStatus) (*clusterv1.Machine, error) {
	// Avoid status within status within status ...
	status.ObjectMeta.Annotations[InstanceStatusAnnotationKey] = ""

	serializer := json.NewSerializer(json.DefaultMetaFactory, ext.scheme, ext.scheme, false)
	b := []byte{}
	buff := bytes.NewBuffer(b)
	err := serializer.Encode((*clusterv1.Machine)(status), buff)
	if err != nil {
		return nil, fmt.Errorf("encoding failure: %v", err)
	}

	if machine.ObjectMeta.Annotations == nil {
		machine.ObjectMeta.Annotations = make(map[string]string)
	}
	machine.ObjectMeta.Annotations[InstanceStatusAnnotationKey] = buff.String()
	return machine, nil
}
