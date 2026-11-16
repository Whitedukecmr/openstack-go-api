package cloud

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/volumes"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/flavors"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/keypairs"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/v2/openstack/image/v2/images"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/networks"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/subnets"
)

// OpenStack implements Cloud with gophercloud.
type OpenStack struct {
	compute *gophercloud.ServiceClient
	image   *gophercloud.ServiceClient
	network *gophercloud.ServiceClient
	volume  *gophercloud.ServiceClient // nil if Cinder is not deployed
}

// NewOpenStack authenticates against Keystone using the OS_* environment
// variables (source devstack/openrc admin admin).
func NewOpenStack(ctx context.Context) (*OpenStack, error) {
	ao, err := authOptionsFromEnv()
	if err != nil {
		return nil, fmt.Errorf("load credentials: %w", err)
	}
	eo := gophercloud.EndpointOpts{Region: os.Getenv("OS_REGION_NAME")}

	provider, err := openstack.AuthenticatedClient(ctx, ao)
	if err != nil {
		return nil, fmt.Errorf("keystone auth: %w", err)
	}

	o := &OpenStack{}
	if o.compute, err = openstack.NewComputeV2(provider, eo); err != nil {
		return nil, fmt.Errorf("nova: %w", err)
	}
	o.compute.Microversion = "2.47" // flavor embedded in server details
	if o.image, err = openstack.NewImageV2(provider, eo); err != nil {
		return nil, fmt.Errorf("glance: %w", err)
	}
	if o.network, err = openstack.NewNetworkV2(provider, eo); err != nil {
		return nil, fmt.Errorf("neutron: %w", err)
	}
	if v, err := openstack.NewBlockStorageV3(provider, eo); err == nil {
		o.volume = v
	}
	return o, nil
}

func mapErr(err error) error {
	if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
		return ErrNotFound
	}
	return err
}

// ---------- Compute ----------

func toServer(s servers.Server) Server {
	out := Server{ID: s.ID, Name: s.Name, Status: s.Status, Created: s.Created, Addresses: map[string][]string{}}
	if id, ok := s.Image["id"].(string); ok {
		out.ImageID = id
	}
	if id, ok := s.Flavor["id"].(string); ok {
		out.FlavorID = id
	} else if name, ok := s.Flavor["original_name"].(string); ok {
		out.FlavorID = name
	}
	for net, raw := range s.Addresses {
		list, _ := raw.([]any)
		for _, a := range list {
			if m, ok := a.(map[string]any); ok {
				if ip, ok := m["addr"].(string); ok {
					out.Addresses[net] = append(out.Addresses[net], ip)
				}
			}
		}
	}
	return out
}

func (o *OpenStack) ListServers(ctx context.Context) ([]Server, error) {
	pages, err := servers.List(o.compute, servers.ListOpts{}).AllPages(ctx)
	if err != nil {
		return nil, err
	}
	all, err := servers.ExtractServers(pages)
	if err != nil {
		return nil, err
	}
	out := make([]Server, 0, len(all))
	for _, s := range all {
		out = append(out, toServer(s))
	}
	return out, nil
}

func (o *OpenStack) GetServer(ctx context.Context, id string) (*Server, error) {
	s, err := servers.Get(ctx, o.compute, id).Extract()
	if err != nil {
		return nil, mapErr(err)
	}
	srv := toServer(*s)
	return &srv, nil
}

func (o *OpenStack) CreateServer(ctx context.Context, req CreateServerRequest) (*Server, error) {
	var opts servers.CreateOptsBuilder = servers.CreateOpts{
		Name:      req.Name,
		FlavorRef: req.FlavorID,
		ImageRef:  req.ImageID,
		Networks:  []servers.Network{{UUID: req.NetworkID}},
	}
	if req.KeyName != "" {
		opts = keypairs.CreateOptsExt{CreateOptsBuilder: opts, KeyName: req.KeyName}
	}
	s, err := servers.Create(ctx, o.compute, opts, nil).Extract()
	if err != nil {
		return nil, err
	}
	srv := toServer(*s)
	srv.Name = req.Name
	return &srv, nil
}

func (o *OpenStack) DeleteServer(ctx context.Context, id string) error {
	return mapErr(servers.Delete(ctx, o.compute, id).ExtractErr())
}

func (o *OpenStack) ServerAction(ctx context.Context, id, action string) error {
	var err error
	switch action {
	case "start":
		err = servers.Start(ctx, o.compute, id).ExtractErr()
	case "stop":
		err = servers.Stop(ctx, o.compute, id).ExtractErr()
	case "reboot":
		err = servers.Reboot(ctx, o.compute, id, servers.RebootOpts{Type: servers.SoftReboot}).ExtractErr()
	default:
		return fmt.Errorf("unsupported action %q", action)
	}
	return mapErr(err)
}

func (o *OpenStack) ListFlavors(ctx context.Context) ([]Flavor, error) {
	pages, err := flavors.ListDetail(o.compute, flavors.ListOpts{}).AllPages(ctx)
	if err != nil {
		return nil, err
	}
	all, err := flavors.ExtractFlavors(pages)
	if err != nil {
		return nil, err
	}
	out := make([]Flavor, 0, len(all))
	for _, f := range all {
		out = append(out, Flavor{ID: f.ID, Name: f.Name, VCPUs: f.VCPUs, RAM: f.RAM, Disk: f.Disk})
	}
	return out, nil
}

// ---------- Image ----------

func (o *OpenStack) ListImages(ctx context.Context) ([]Image, error) {
	pages, err := images.List(o.image, images.ListOpts{}).AllPages(ctx)
	if err != nil {
		return nil, err
	}
	all, err := images.ExtractImages(pages)
	if err != nil {
		return nil, err
	}
	out := make([]Image, 0, len(all))
	for _, i := range all {
		out = append(out, Image{ID: i.ID, Name: i.Name, Status: string(i.Status), SizeMB: i.SizeBytes / (1 << 20)})
	}
	return out, nil
}

// ---------- Network ----------

func (o *OpenStack) ListNetworks(ctx context.Context) ([]Network, error) {
	pages, err := networks.List(o.network, networks.ListOpts{}).AllPages(ctx)
	if err != nil {
		return nil, err
	}
	all, err := networks.ExtractNetworks(pages)
	if err != nil {
		return nil, err
	}
	out := make([]Network, 0, len(all))
	for _, n := range all {
		out = append(out, Network{ID: n.ID, Name: n.Name, Status: n.Status, Subnets: n.Subnets})
	}
	return out, nil
}

func (o *OpenStack) CreateNetwork(ctx context.Context, req CreateNetworkRequest) (*Network, error) {
	n, err := networks.Create(ctx, o.network, networks.CreateOpts{Name: req.Name}).Extract()
	if err != nil {
		return nil, err
	}
	sn, err := subnets.Create(ctx, o.network, subnets.CreateOpts{
		NetworkID: n.ID,
		Name:      req.Name + "-subnet",
		CIDR:      req.CIDR,
		IPVersion: gophercloud.IPv4,
	}).Extract()
	if err != nil {
		// rollback to avoid orphan networks
		_ = networks.Delete(ctx, o.network, n.ID).ExtractErr()
		return nil, fmt.Errorf("create subnet: %w", err)
	}
	return &Network{ID: n.ID, Name: n.Name, Status: n.Status, Subnets: []string{sn.ID}}, nil
}

func (o *OpenStack) DeleteNetwork(ctx context.Context, id string) error {
	return mapErr(networks.Delete(ctx, o.network, id).ExtractErr())
}

// ---------- Block storage ----------

var errNoCinder = errors.New("block storage (cinder) not available in this cloud")

func (o *OpenStack) ListVolumes(ctx context.Context) ([]Volume, error) {
	if o.volume == nil {
		return nil, errNoCinder
	}
	pages, err := volumes.List(o.volume, volumes.ListOpts{}).AllPages(ctx)
	if err != nil {
		return nil, err
	}
	all, err := volumes.ExtractVolumes(pages)
	if err != nil {
		return nil, err
	}
	out := make([]Volume, 0, len(all))
	for _, v := range all {
		out = append(out, Volume{ID: v.ID, Name: v.Name, SizeGB: v.Size, Status: v.Status})
	}
	return out, nil
}

func (o *OpenStack) CreateVolume(ctx context.Context, req CreateVolumeRequest) (*Volume, error) {
	if o.volume == nil {
		return nil, errNoCinder
	}
	v, err := volumes.Create(ctx, o.volume, volumes.CreateOpts{Name: req.Name, Size: req.SizeGB}, nil).Extract()
	if err != nil {
		return nil, err
	}
	return &Volume{ID: v.ID, Name: v.Name, SizeGB: v.Size, Status: v.Status}, nil
}

func (o *OpenStack) DeleteVolume(ctx context.Context, id string) error {
	if o.volume == nil {
		return errNoCinder
	}
	return mapErr(volumes.Delete(ctx, o.volume, id, volumes.DeleteOpts{}).ExtractErr())
}
