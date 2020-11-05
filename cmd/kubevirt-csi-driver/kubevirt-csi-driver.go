package main

import (
	"context"
	"flag"
	"io/ioutil"
	"math/rand"
	"net/url"
	"os"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	certutil "k8s.io/client-go/util/cert"

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
}

func main() {
	flag.Parse()
	rand.Seed(time.Now().UnixNano())
	handle()
	os.Exit(0)
}

func handle() {
	if service.VendorVersion == "" {
		log.Fatalf("VendorVersion must be set at compile time")
	}
	log.Infof("Driver vendor %v %v", service.VendorName, service.VendorVersion)

	log.Infof("apiUrl %v", *infraClusterApiUrl)
	infraClusterConfig, err := buildInfraClusterConfig(*infraClusterApiUrl, *infraClusterToken, *infraClusterCA)
	if err != nil {
		log.Fatalf("Failed to build infra cluster config: %v", err)
	}

	infraClusterClientSet, err := kubernetes.NewForConfig(infraClusterConfig)
	if err != nil {
		log.Fatalf("Failed to initialize KubeVirt client: %s", err)
	}

	virtClient, err := kubevirt.NewClient(infraClusterConfig)
	if err != nil {
		log.Fatal(err)
	}

	// TODO revise the assumption that the  current running node name should be the infracluster VM name.
	if *nodeName != "" {
		_, err = virtClient.GetVMI(context.Background(), *infraClusterNamespace, *nodeName)
		if err != nil {
			log.Fatalf("failed to find a VM in the infra cluster with that name %v: %v", *nodeName, err)
		}
	}

	driver := service.NewkubevirtCSIDriver(*infraClusterClientSet, virtClient, *nodeName)

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
		log.Errorf("Expected to load root CA config from %s, but got err: %v", caFile, err)
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
