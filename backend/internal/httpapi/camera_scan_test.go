package httpapi

import "testing"

func TestExpandCameraScanHostsAllowsPrivateCIDR(t *testing.T) {
	hosts, err := expandCameraScanHosts(cameraScanRequest{CIDR: "192.168.1.0/30"})
	if err != nil {
		t.Fatalf("expected private cidr to be accepted: %v", err)
	}
	if len(hosts) != 4 {
		t.Fatalf("expected 4 hosts, got %d", len(hosts))
	}
}

func TestExpandCameraScanHostsRejectsPublicRange(t *testing.T) {
	_, err := expandCameraScanHosts(cameraScanRequest{StartIP: "8.8.8.8", EndIP: "8.8.8.9"})
	if err == nil {
		t.Fatalf("expected public range to be rejected")
	}
}

func TestNormalizeCameraScanPortsDedupesAndSorts(t *testing.T) {
	ports, err := normalizeCameraScanPorts([]int{8899, 80, 80})
	if err != nil {
		t.Fatalf("normalize ports: %v", err)
	}
	if len(ports) != 2 || ports[0] != 80 || ports[1] != 8899 {
		t.Fatalf("unexpected ports: %#v", ports)
	}
}

func TestHostFromRTSPURLExtractsIPWithoutCredentials(t *testing.T) {
	host := hostFromRTSPURL("rtsp://admin:secret@192.168.1.57:554/cam/realmonitor?channel=1")
	if host != "192.168.1.57" {
		t.Fatalf("unexpected host: %q", host)
	}
}
