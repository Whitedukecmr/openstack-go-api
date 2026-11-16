// Package cloud defines the domain model and the Cloud interface used by the
// HTTP layer. The OpenStack implementation lives in openstack.go; tests use a fake.
package cloud

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned when the requested resource does not exist.
var ErrNotFound = errors.New("resource not found")

type Server struct {
	ID        string              `json:"id"`
	Name      string              `json:"name"`
	Status    string              `json:"status"`
	FlavorID  string              `json:"flavor_id,omitempty"`
	ImageID   string              `json:"image_id,omitempty"`
	Addresses map[string][]string `json:"addresses,omitempty"`
	Created   time.Time           `json:"created"`
}

type CreateServerRequest struct {
	Name      string `json:"name"`
	FlavorID  string `json:"flavor_id"`
	ImageID   string `json:"image_id"`
	NetworkID string `json:"network_id"`
	KeyName   string `json:"key_name,omitempty"`
}

type Flavor struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	VCPUs int    `json:"vcpus"`
	RAM   int    `json:"ram_mb"`
	Disk  int    `json:"disk_gb"`
}

type Image struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	SizeMB int64  `json:"size_mb"`
}

type Network struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Status  string   `json:"status"`
	Subnets []string `json:"subnets"`
}

type CreateNetworkRequest struct {
	Name string `json:"name"`
	CIDR string `json:"cidr"`
}

type Volume struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	SizeGB int    `json:"size_gb"`
	Status string `json:"status"`
}

type CreateVolumeRequest struct {
	Name   string `json:"name"`
	SizeGB int    `json:"size_gb"`
}

// Cloud is everything the API needs from the IaaS backend.
type Cloud interface {
	ListServers(ctx context.Context) ([]Server, error)
	GetServer(ctx context.Context, id string) (*Server, error)
	CreateServer(ctx context.Context, req CreateServerRequest) (*Server, error)
	DeleteServer(ctx context.Context, id string) error
	ServerAction(ctx context.Context, id, action string) error

	ListFlavors(ctx context.Context) ([]Flavor, error)
	ListImages(ctx context.Context) ([]Image, error)

	ListNetworks(ctx context.Context) ([]Network, error)
	CreateNetwork(ctx context.Context, req CreateNetworkRequest) (*Network, error)
	DeleteNetwork(ctx context.Context, id string) error

	ListVolumes(ctx context.Context) ([]Volume, error)
	CreateVolume(ctx context.Context, req CreateVolumeRequest) (*Volume, error)
	DeleteVolume(ctx context.Context, id string) error
}
