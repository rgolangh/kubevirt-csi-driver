package service

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
)

func TestDeviceExtraction(t *testing.T) {

	device, err := getDeviceBySerialID("S35ENX0J663758")
	t.Log(err)
	t.Logf("device %+v", device)

}
