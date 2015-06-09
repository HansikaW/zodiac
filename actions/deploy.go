package actions

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/CenturyLinkLabs/prettycli"
	"github.com/CenturyLinkLabs/zodiac/cluster"
	"github.com/CenturyLinkLabs/zodiac/composer"
	"github.com/CenturyLinkLabs/zodiac/proxy"
	"github.com/samalba/dockerclient"
)

const ProxyAddress = "localhost:31981"

var (
	DefaultProxy    proxy.Proxy
	DefaultComposer composer.Composer
)

func init() {
	DefaultProxy = proxy.NewHTTPProxy(ProxyAddress)
	DefaultComposer = composer.NewExecComposer(ProxyAddress)
}

type DeploymentManifests []DeploymentManifest

type DeploymentManifest struct {
	Services   []Service
	DeployedAt string
}

type Service struct {
	Name            string
	ContainerConfig dockerclient.ContainerConfig
}

func Deploy(c cluster.Cluster, args []string) (prettycli.Output, error) {

	var reqs []cluster.ContainerRequest

	for _, endpoint := range c.Endpoints() {

		//client, _ := dockerclient.NewDockerClient(endpoint.Name(), nil)
		// TODO: handle error
		go DefaultProxy.Serve(endpoint)
		// TODO: handle error
		defer DefaultProxy.Stop()

		// TODO: handle error
		// TODO: args not passed!
		DefaultComposer.Run(args)
		reqs = DefaultProxy.DrainRequests()

		dm := DeploymentManifest{
			Services:   []Service{},
			DeployedAt: time.Now().Format(time.RFC3339),
		}

		for _, req := range reqs {
			s, err := serviceForRequest(req)
			if err != nil {
				return nil, err
			}

			imageId, err := endpoint.ResolveImage(s.ContainerConfig.Image)
			if err != nil {
				return nil, err
			}

			s.ContainerConfig.Image = imageId

			dm.Services = append(dm.Services, s)
		}

		oldManifestBlob := "[]"
		for _, svc := range dm.Services {
			ci, err := endpoint.InspectContainer(svc.Name)

			if err == nil {
				err := endpoint.RemoveContainer(svc.Name)
				if err != nil {
					//TODO: figure out if we really want to abort here
					return nil, err
				}
			}
			// TODO, only assign if not empty
			oldManifestBlob = ci.Config.Labels["zodiacManifest"]
		}

		var dms DeploymentManifests
		if err := json.Unmarshal([]byte(oldManifestBlob), &dms); err != nil {
			return nil, err
		}
		dms = append(dms, dm)

		manifestsBlob, err := json.Marshal(dms)
		if err != nil {
			return nil, err
		}

		//for _, req := range reqs {
		for _, svc := range dm.Services {

			if svc.ContainerConfig.Labels == nil {
				svc.ContainerConfig.Labels = make(map[string]string)
			}
			svc.ContainerConfig.Labels["zodiacManifest"] = string(manifestsBlob)
			// Phase Deux: Build current manifest

			// Phase Deux: Fetch current deployments

			// Phase Deux: Build new Manifests from ContainerRequest + Old Manifest

			// Phase Un: (Pull?)+Create+Start Containers on all hosts
			// TODO: handle error

			//endpoint.StartContainers(DefaultProxy.DrainRequests())
			endpoint.StartContainer(svc.Name, svc.ContainerConfig)
			// Phase Deux: ^ injecting manifest before Create
		}
	}

	output := fmt.Sprintf("Successfully deployed %d container(s)", len(reqs))
	return prettycli.PlainOutput{output}, nil
}

func serviceForRequest(req cluster.ContainerRequest) (Service, error) {
	var cc dockerclient.ContainerConfig

	if err := json.Unmarshal(req.CreateOptions, &cc); err != nil {
		return Service{}, err
	}

	return Service{
		Name:            req.Name,
		ContainerConfig: cc,
	}, nil
}
