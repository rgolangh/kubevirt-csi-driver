package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/url"
	"os"
	"time"

	"github.com/pkg/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	certutil "k8s.io/client-go/util/cert"
	"k8s.io/klog"

	"github.com/kubevirt/csi-driver/pkg/kubevirt"
	"github.com/kubevirt/csi-driver/pkg/service"
)

var (
	endpoint               = flag.String("endpoint", "unix:/csi/csi.sock", "CSI endpoint")
	namespace              = flag.String("namespace", "", "Namespace to run the controllers on")
	nodeName               = flag.String("node-name", "", "The node name - the node this pods runs on")
	infraClusterNamespace  = flag.String("infra-cluster-namespace", "", "The infra-cluster namespace")
	infraClusterApiUrl     = flag.String("infra-cluster-api-url", "", "The infra-cluster API URL")
	infraClusterToken      = flag.String("infra-cluster-token", "", "The infra-cluster token file")
	infraClusterCA         = flag.String("infra-cluster-ca", "", "the infra-cluster ca certificate file")
)

func init() {
	flag.Set("logtostderr", "true")
	// make glog and klog coexist
	klogFlags := flag.NewFlagSet("klog", flag.ExitOnError)
	klog.Info("nice to meet you")
	klog.InitFlags(klogFlags)

	// Sync the glog and klog flags.
	flag.CommandLine.VisitAll(func(f1 *flag.Flag) {
		f2 := klogFlags.Lookup(f1.Name)
		if f2 != nil {
			value := f1.Value.String()
			f2.Value.Set(value)
		}
	})
}

func main() {
	flag.Parse()
	rand.Seed(time.Now().UnixNano())
	handle()
	os.Exit(0)
}

func handle() {
	if service.VendorVersion == "" {
		klog.Fatalf("VendorVersion must be set at compile time")
	}
	klog.V(2).Infof("Driver vendor %v %v", service.VendorName, service.VendorVersion)

	tenantConfig, err := rest.InClusterConfig()
	if err != nil {
		klog.Fatalf("Failed to build tenant cluster config: %v", err)
	}

	tenantClientSet, err := kubernetes.NewForConfig(tenantConfig)
	if err != nil {
		klog.Fatalf("Failed to build tenant client set: %v", err)
	}

	infraClusterConfig, err := buildInfraClusterConfig(*infraClusterApiUrl, *infraClusterToken, *infraClusterCA)
	if err != nil {
		klog.Fatalf("Failed to build infra cluster config: %v", err)
	}

	infraClusterClientSet, err := kubernetes.NewForConfig(infraClusterConfig)
	if err != nil {
		klog.Fatalf("Failed to initialize KubeVirt client: %s", err)
	}

	virtClient, err := kubevirt.NewClient(infraClusterConfig)
	if err != nil {
		klog.Fatal(err)
	}

	var nodeId string
	if *nodeName != "" {
		node, err := tenantClientSet.CoreV1().Nodes().Get(*nodeName, v1.GetOptions{})
		if err != nil {
			klog.Fatal(fmt.Errorf("failed to find node by name %v: %v", nodeName, err))
		}
		// systemUUID is the VM ID
		nodeId = node.Status.NodeInfo.SystemUUID
	}

	driver := service.NewkubevirtCSIDriver(*infraClusterClientSet, virtClient, *infraClusterNamespace, nodeId)

	driver.Run(*endpoint)
}

func buildInfraClusterConfig(apiUrl string, tokenFile string, caFile string) (*rest.Config, error){
	parse, err := url.Parse(apiUrl)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to parse apiUrl %v", apiUrl)
	}

	token, err := ioutil.ReadFile(tokenFile)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to read tokenFile %v", tokenFile)
	}

	tlsClientConfig := rest.TLSClientConfig{}
	if _, err := certutil.NewPool(caFile); err != nil {
		klog.Errorf("Expected to load root CA config from %s, but got err: %v", caFile, err)
	} else {
		tlsClientConfig.CAFile = caFile
	}


	return &rest.Config{
		Host: parse.Host,
		TLSClientConfig: tlsClientConfig,
		BearerToken:     string(token),
		BearerTokenFile: tokenFile,
	}, nil
}
