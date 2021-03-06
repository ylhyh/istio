// Copyright 2017 Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package external

import (
	"net"
	"strings"

	meshconfig "istio.io/api/mesh/v1alpha1"
	networking "istio.io/api/networking/v1alpha3"
	"istio.io/istio/pilot/pkg/model"
)

func convertPort(port *networking.Port) *model.Port {
	return &model.Port{
		Name:                 port.Name,
		Port:                 int(port.Number),
		Protocol:             model.ParseProtocol(port.Protocol),
		AuthenticationPolicy: meshconfig.AuthenticationPolicy_NONE,
	}
}

func convertServices(externalService *networking.ExternalService) []*model.Service {
	out := make([]*model.Service, 0)

	var resolution model.Resolution
	switch externalService.Discovery {
	case networking.ExternalService_NONE:
		resolution = model.Passthrough
	case networking.ExternalService_DNS:
		resolution = model.DNSLB
	case networking.ExternalService_STATIC:
		resolution = model.ClientSideLB
	}

	svcPorts := make(model.PortList, 0, len(externalService.Ports))
	for _, port := range externalService.Ports {
		svcPorts = append(svcPorts, convertPort(port))
	}

	for _, host := range externalService.Hosts {
		// set address if host is an IP or CIDR prefix
		var address string
		if _, _, cidrErr := net.ParseCIDR(host); cidrErr == nil || net.ParseIP(host) != nil {
			address = host
			// FIXME: create common function for CIDR prefix to metrics friendly name?
			host = strings.Replace(host, "/", "_", -1) // make hostname easy to parse for metrics
		}

		out = append(out, &model.Service{
			MeshExternal: true,
			Hostname:     host,
			Address:      address,
			Ports:        svcPorts,
			Resolution:   resolution,
		})
	}

	return out
}

func convertEndpoint(service *model.Service, servicePort *networking.Port,
	endpoint *networking.ExternalService_Endpoint) *model.ServiceInstance {

	instancePort := endpoint.Ports[servicePort.Name]
	if instancePort == 0 {
		instancePort = servicePort.Number
	}

	return &model.ServiceInstance{
		Endpoint: model.NetworkEndpoint{
			Address:     endpoint.Address,
			Port:        int(instancePort),
			ServicePort: convertPort(servicePort),
		},
		// TODO AvailabilityZone, ServiceAccount
		Service: service,
		Labels:  endpoint.Labels,
	}
}

func convertInstances(externalService *networking.ExternalService) []*model.ServiceInstance {
	out := make([]*model.ServiceInstance, 0)
	for _, service := range convertServices(externalService) {
		for _, servicePort := range externalService.Ports {
			if len(externalService.Endpoints) == 0 &&
				externalService.Discovery == networking.ExternalService_DNS {
				// when external service has discovery type DNS and no endpoints
				// we create endpoints from external service hosts field
				for _, host := range externalService.Hosts {
					out = append(out, &model.ServiceInstance{
						Endpoint: model.NetworkEndpoint{
							Address:     host,
							Port:        int(servicePort.Number),
							ServicePort: convertPort(servicePort),
						},
						// TODO AvailabilityZone, ServiceAccount
						Service: service,
						Labels:  nil,
					})
				}
			}
			for _, endpoint := range externalService.Endpoints {
				out = append(out, convertEndpoint(service, servicePort, endpoint))
			}
		}
	}
	return out
}
