package service

import (
	"strconv"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"k8s.io/client-go/kubernetes"

	"github.com/kubevirt/csi-driver/pkg/kubevirt"

	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"
)

const (
	ParameterThinProvisioning  = "thinProvisioning"
)

//ControllerService implements the controller interface
type ControllerService struct {
	infraClusterClient kubernetes.Clientset
	kubevirtClient     kubevirt.Client
}

var ControllerCaps = []csi.ControllerServiceCapability_RPC_Type{
	csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
	csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME, // attach/detach
}

//CreateVolume creates the disk for the request, unattached from any VM
func (c *ControllerService) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	klog.Infof("Creating disk %s", req.Name)

	//1. idempotence first - see if disk already exists, kubevirt creates disk by name(alias in kubevirt as well)
	//c.kubevirtClient.ListDataVolumeNames(req.GetName())

	// 2. create the data volume if it doesn't exist.
	// TODO kubevirt client needs a Creat function.

	// TODO support for thin/thick provisioning from the storage class parameters
	_, _ = strconv.ParseBool(req.Parameters[ParameterThinProvisioning])

	// 3. return a response TODO stub values for now
	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			CapacityBytes:      1024,
			VolumeId:           "uuidofthedatavolume",
		},
	}, nil
}

//DeleteVolume removed the data volume from kubevirt
func (c *ControllerService) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	klog.Infof("Removing data volume with ID %s", req.VolumeId)

	// 1. get data volume name by uid by filtering the list of all volumes of namespace. (there is not getById)
	// err := c.kubevirtClient.ListDataVolumeNames()

	// 2. delete the volume
	err := c.kubevirtClient.DeleteDataVolume("fill this", "dvname")
	return &csi.DeleteVolumeResponse{}, err
}

// ControllerPublishVolume takes a volume, which is an kubevirt disk, and attaches it to a node, which is an kubevirt VM.
func (c *ControllerService) ControllerPublishVolume(
	ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {

	// req.NodeId == kubevirt VM name
	klog.Infof("Attaching DataVolume %s to VM %s", req.VolumeId, req.NodeId)

	// 1. get DataVolume by ID

	// 2. hotplug DataVolume to VMI using subresource - see virtctl/addvolume for reference

	return &csi.ControllerPublishVolumeResponse{}, nil
}

//ControllerUnpublishVolume detaches the disk from the VM.
func (c *ControllerService) ControllerUnpublishVolume(_ context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	// req.NodeId == kubevirt VM name
	klog.Infof("Detaching DataVolume %s from VM %s", req.VolumeId, req.NodeId)

	// 1. get DataVolume by ID

	// 2. hot-unplug DataVolume to VMI using subresource - see virtctl/removevolume for reference

	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

//ValidateVolumeCapabilities
func (c *ControllerService) ValidateVolumeCapabilities(context.Context, *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

//ListVolumes
func (c *ControllerService) ListVolumes(context.Context, *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

//GetCapacity
func (c *ControllerService) GetCapacity(context.Context, *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

//CreateSnapshot
func (c *ControllerService) CreateSnapshot(context.Context, *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

//DeleteSnapshot
func (c *ControllerService) DeleteSnapshot(context.Context, *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

//ListSnapshots
func (c *ControllerService) ListSnapshots(context.Context, *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

//ControllerExpandVolume
func (c *ControllerService) ControllerExpandVolume(context.Context, *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

//ControllerGetCapabilities
func (c *ControllerService) ControllerGetCapabilities(context.Context, *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	caps := make([]*csi.ControllerServiceCapability, 0, len(ControllerCaps))
	for _, capability := range ControllerCaps {
		caps = append(
			caps,
			&csi.ControllerServiceCapability{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: capability,
					},
				},
			},
		)
	}
	return &csi.ControllerGetCapabilitiesResponse{Capabilities: caps}, nil
}

func (c *ControllerService) ControllerGetVolume(ctx context.Context, request *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {

	return &csi.ControllerGetVolumeResponse{
		Volume: &csi.Volume{
			CapacityBytes: 0,
			VolumeId:      "TODO",
		},
	}, nil
}