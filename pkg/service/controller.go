package service

import (
	"fmt"
	"strconv"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"k8s.io/client-go/kubernetes"

	"github.com/kubevirt/csi-driver/pkg/kubevirt"

	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog"
	v1 "kubevirt.io/client-go/api/v1"
	cdiv1 "kubevirt.io/containerized-data-importer/pkg/apis/core/v1alpha1"
)

const (
	ParameterThinProvisioning      = "thinProvisioning"
	infraStorageClassNameParameter = "infraStorageClassName"
	busParameter                   = "bus"
)

//ControllerService implements the controller interface
type ControllerService struct {
	infraClusterNamespace string
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

	// Create DataVolume object
	// Create DataVolume resource in infra cluster
	// Get details of new DataVolume resource
	// Wait until DataVolume is ready??
	dv := cdiv1.DataVolume{}

	storageClassName := req.Parameters[infraStorageClassNameParameter]
	volumeMode := corev1.PersistentVolumeFilesystem // TODO: get it from req.VolumeCapabilities
	dv.Name = req.Name
	dv.Spec.PVC = &corev1.PersistentVolumeClaimSpec{
		AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
		StorageClassName: &storageClassName,
		VolumeMode:       &volumeMode,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceRequestsStorage: *resource.NewScaledQuantity(req.GetCapacityRange().GetRequiredBytes(), 0)},
		},
	}
	bus := req.Parameters[busParameter]

	//1. idempotence first - see if disk already exists, kubevirt creates disk by name(alias in kubevirt as well)
	names, err := c.kubevirtClient.ListDataVolumeNames(c.infraClusterNamespace, map[string]string{})
	if err != nil {
		return nil, err
	}
	for _, dv := range names {
		if dv.Name == req.Name {
			// dv exists, nothing more to do
			return &csi.CreateVolumeResponse{
				Volume: &csi.Volume{
					CapacityBytes: req.GetCapacityRange().GetRequiredBytes(),
					VolumeId:      dv.Name,
					VolumeContext: map[string]string{busParameter: bus},
				},
			}, nil
		}
	}

	// 2. create the data volume if it doesn't exist.
	err = c.kubevirtClient.CreateDataVolume(c.infraClusterNamespace, dv)
	if err != nil {
		klog.Errorf("failed to create data volume on infra-cluster %v", err)
		return nil, err
	}
	// TODO support for thin/thick provisioning from the storage class parameters
	_, _ = strconv.ParseBool(req.Parameters[ParameterThinProvisioning])

	// 3. return a response TODO stub values for now
	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			CapacityBytes: req.GetCapacityRange().GetRequiredBytes(),
			VolumeId:      dv.Name,
			VolumeContext: map[string]string{busParameter: bus},
		},
	}, nil
}

//DeleteVolume removed the data volume from kubevirt
func (c *ControllerService) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	klog.Infof("Removing data volume with ID %s", req.VolumeId)

	// Yaron since we set the VolumeID in CreateVolume then for use the volumeID==dvName,
	// so we don't need the lines here
	//dvName, err := c.getDataVolumeNameByUID(ctx, req.VolumeId)
	//if err != nil {
	//	return nil, err
	//}

	err := c.kubevirtClient.DeleteDataVolume(c.infraClusterNamespace, req.VolumeId)
	return &csi.DeleteVolumeResponse{}, err
}

// ControllerPublishVolume takes a volume, which is an kubevirt disk, and attaches it to a node, which is an kubevirt VM.
func (c *ControllerService) ControllerPublishVolume(
	ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {

	// req.NodeId == kubevirt VM name
	klog.Infof("Attaching DataVolume UID %s to Node ID %s", req.VolumeId, req.NodeId)

	// Get DataVolume name by ID
	dvName, err := c.getDataVolumeNameByUID(ctx, req.VolumeId)
	if err != nil {
		return nil, err
	}

	// Get VM name
	vmName, err := c.getVmNameByCSINodeID(ctx, c.infraClusterNamespace, req.NodeId)
	if err != nil {
		return nil, err
	}

	// Determine disk name (disk-<DataVolume-name>)
	diskName := "disk-" + dvName

	// Determine serial number/string for the new disk
	serial := req.VolumeId[0:20]

	// Determine BUS type
	bus := req.VolumeContext[busParameter]

	// hotplug DataVolume to VM
	klog.Infof("Start attaching DataVolume %s to VM %s. Disk name: %s. Serial: %s. Bus: %s", dvName, vmName, diskName, serial, bus)

	hotplugRequest := v1.HotplugVolumeRequest{
		Volume: &v1.Volume{
			VolumeSource: v1.VolumeSource{
				DataVolume: &v1.DataVolumeSource{
					Name: dvName,
				},
			},
			Name: diskName,
		},
		Disk: &v1.Disk{
			Name:   diskName,
			Serial: serial,
			DiskDevice: v1.DiskDevice{
				Disk: &v1.DiskTarget{
					Bus: bus,
				},
			},
		},
		Ephemeral: false,
	}
	err = c.kubevirtClient.AddVolumeToVM(c.infraClusterNamespace, vmName, hotplugRequest)
	if err != nil {
		return nil, err
	}

	return &csi.ControllerPublishVolumeResponse{}, nil
}

//ControllerUnpublishVolume detaches the disk from the VM.
func (c *ControllerService) ControllerUnpublishVolume(_ context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	// req.NodeId == kubevirt VM name
	klog.Infof("Detaching DataVolume UID %s from Node ID %s", req.VolumeId, req.NodeId)

	// Get DataVolume name by ID
	dvName, err := c.getDataVolumeNameByUID(context.Background(), req.VolumeId)
	if err != nil {
		return nil, err
	}

	// Get VM name
	vmName, err := c.getVmNameByCSINodeID(context.Background(), c.infraClusterNamespace, req.NodeId)
	if err != nil {
		return nil, err
	}

	// Determine disk name (disk-<DataVolume-name>)
	diskName := "disk-" + dvName

	// Detach DataVolume from VM
	hotplugRequest := v1.HotplugVolumeRequest{
		Volume: &v1.Volume{
			VolumeSource: v1.VolumeSource{
				DataVolume: &v1.DataVolumeSource{
					Name: dvName,
				},
			},
			Name: diskName,
		},
	}
	err = c.kubevirtClient.RemoveVolumeFromVM(c.infraClusterNamespace, vmName, hotplugRequest)
	if err != nil {
		return nil, err
	}

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

// TODO I think this is not needed - the CSI volumeID can be the DataVolume name most probably
func (c *ControllerService) getDataVolumeNameByUID(ctx context.Context, uid string) (string, error) {
	//
	//
	//resource := getDvGroupVersionResource()
	//
	//list, err := c.infraClusterClient.Resource(resource).Namespace(InfraNamespace).List(ctx, metav1.ListOptions{})
	//if err != nil {
	//	return "", err
	//}
	//
	//dvName := ""
	//
	//for _, dv := range list.Items {
	//	if string(dv.GetUID()) == uid {
	//		dvName = dv.GetName()
	//		break
	//	}
	//}
	//
	//if dvName == "" {
	//	return "", status.Error(codes.NotFound, "DataVolume uid: "+uid)
	//}
	//
	//return dvName, nil
	return uid, nil
}

// getVmNameByCSINodeID find a VM in infra cluster by its firmware uuid. The uid is the ID that the CSI node
// part publishes in NodeGetInfo and then used by CSINode.spec.drivers[].nodeID
func (c *ControllerService) getVmNameByCSINodeID(_ context.Context,namespace string, csiNodeID string) (string, error) {
	vmis, err := c.kubevirtClient.ListVirtualMachines(namespace, map[string]string{})
	if err != nil {
		klog.Errorf("failed to list VMIS %v", err)
		return "", err
	}

	for _, vmi := range vmis {
		if string(vmi.Spec.Domain.Firmware.UUID) == csiNodeID {
			return vmi.Name, nil
		}
	}
	return "", fmt.Errorf("failed to find VM with domain.firmware.uuid %v", csiNodeID)
}

func getNodesGroupVersionResource() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    cdiv1.SchemeGroupVersion.Group,
		Version:  cdiv1.SchemeGroupVersion.Version,
		Resource: "nodes",
	}
}

func getDvGroupVersionResource() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    cdiv1.SchemeGroupVersion.Group,
		Version:  cdiv1.SchemeGroupVersion.Version,
		Resource: "datavolumes",
	}
}
