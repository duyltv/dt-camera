package httpapi

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	maxCameraScanHosts   = 512
	maxCameraScanTargets = 2048
)

var defaultONVIFPorts = []int{80, 8899, 8000, 8080}

type cameraScanRequest struct {
	CIDR      string `json:"cidr,omitempty"`
	StartIP   string `json:"start_ip,omitempty"`
	EndIP     string `json:"end_ip,omitempty"`
	Ports     []int  `json:"ports,omitempty"`
	TimeoutMS int    `json:"timeout_ms,omitempty"`
}

type cameraScanDeviceResponse struct {
	IP                 string `json:"ip"`
	Port               int    `json:"port"`
	XAddr              string `json:"xaddr"`
	ONVIF              bool   `json:"onvif"`
	ExistingCameraID   string `json:"existing_camera_id,omitempty"`
	ExistingCameraName string `json:"existing_camera_name,omitempty"`
	AuthRequired       bool   `json:"auth_required,omitempty"`
	Manufacturer       string `json:"manufacturer,omitempty"`
	Model              string `json:"model,omitempty"`
	FirmwareVersion    string `json:"firmware_version,omitempty"`
	SerialNumber       string `json:"serial_number,omitempty"`
	HardwareID         string `json:"hardware_id,omitempty"`
	Status             string `json:"status"`
}

type cameraScanSummaryResponse struct {
	HostsScanned int `json:"hosts_scanned"`
	PortsScanned int `json:"ports_scanned"`
	Targets      int `json:"targets"`
	Found        int `json:"found"`
}

func (s *Server) handleCameraScan(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}

	var req cameraScanRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", err.Error())
		return
	}
	hosts, err := expandCameraScanHosts(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	ports, err := normalizeCameraScanPorts(req.Ports)
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	if len(hosts)*len(ports) > maxCameraScanTargets {
		writeError(w, http.StatusBadRequest, "validation_error", fmt.Sprintf("scan is limited to %d host:port targets", maxCameraScanTargets), nil)
		return
	}

	timeout := time.Duration(req.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 900 * time.Millisecond
	}
	if timeout > 5*time.Second {
		timeout = 5 * time.Second
	}

	devices := scanONVIFTargets(r.Context(), hosts, ports, timeout)
	if err := s.annotateExistingScannedCameras(r.Context(), devices); err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"devices": devices,
		"summary": cameraScanSummaryResponse{
			HostsScanned: len(hosts),
			PortsScanned: len(ports),
			Targets:      len(hosts) * len(ports),
			Found:        len(devices),
		},
	})
}

func expandCameraScanHosts(req cameraScanRequest) ([]net.IP, error) {
	if strings.TrimSpace(req.CIDR) != "" {
		ip, network, err := net.ParseCIDR(strings.TrimSpace(req.CIDR))
		if err != nil {
			return nil, fmt.Errorf("cidr must be a valid IPv4 CIDR")
		}
		if ip.To4() == nil {
			return nil, fmt.Errorf("only IPv4 camera scans are supported")
		}
		var hosts []net.IP
		for value := ipToUint32(network.IP.Mask(network.Mask)); network.Contains(uint32ToIP(value)); value++ {
			host := uint32ToIP(value)
			if isAllowedCameraScanIP(host) {
				hosts = append(hosts, append(net.IP(nil), host...))
			}
			if len(hosts) > maxCameraScanHosts {
				return nil, fmt.Errorf("scan is limited to %d hosts", maxCameraScanHosts)
			}
			if value == ^uint32(0) {
				break
			}
		}
		if len(hosts) == 0 {
			return nil, fmt.Errorf("cidr must include private or local IPv4 hosts")
		}
		return hosts, nil
	}

	start := net.ParseIP(strings.TrimSpace(req.StartIP)).To4()
	end := net.ParseIP(strings.TrimSpace(req.EndIP)).To4()
	if start == nil || end == nil {
		return nil, fmt.Errorf("provide either cidr or start_ip and end_ip")
	}
	startValue := ipToUint32(start)
	endValue := ipToUint32(end)
	if endValue < startValue {
		return nil, fmt.Errorf("end_ip must be greater than or equal to start_ip")
	}
	var hosts []net.IP
	for value := startValue; value <= endValue; value++ {
		host := uint32ToIP(value)
		if !isAllowedCameraScanIP(host) {
			return nil, fmt.Errorf("camera scan is limited to private or local IPv4 ranges")
		}
		hosts = append(hosts, append(net.IP(nil), host...))
		if len(hosts) > maxCameraScanHosts {
			return nil, fmt.Errorf("scan is limited to %d hosts", maxCameraScanHosts)
		}
		if value == ^uint32(0) {
			break
		}
	}
	return hosts, nil
}

func normalizeCameraScanPorts(ports []int) ([]int, error) {
	if len(ports) == 0 {
		return append([]int(nil), defaultONVIFPorts...), nil
	}
	seen := map[int]struct{}{}
	var result []int
	for _, port := range ports {
		if port <= 0 || port > 65535 {
			return nil, fmt.Errorf("ports must be between 1 and 65535")
		}
		if _, ok := seen[port]; ok {
			continue
		}
		seen[port] = struct{}{}
		result = append(result, port)
	}
	if len(result) > 8 {
		return nil, fmt.Errorf("scan is limited to 8 ports")
	}
	sort.Ints(result)
	return result, nil
}

func scanONVIFTargets(ctx context.Context, hosts []net.IP, ports []int, timeout time.Duration) []cameraScanDeviceResponse {
	type target struct {
		ip   net.IP
		port int
	}
	targets := make(chan target)
	results := make(chan cameraScanDeviceResponse)
	var wg sync.WaitGroup
	workerCount := 48
	if len(hosts)*len(ports) < workerCount {
		workerCount = len(hosts) * len(ports)
	}
	if workerCount < 1 {
		workerCount = 1
	}

	client := &http.Client{Timeout: timeout}
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for target := range targets {
				if device, ok := probeONVIFTarget(ctx, client, target.ip.String(), target.port); ok {
					results <- device
				}
			}
		}()
	}

	go func() {
		for _, host := range hosts {
			for _, port := range ports {
				targets <- target{ip: host, port: port}
			}
		}
		close(targets)
		wg.Wait()
		close(results)
	}()

	devices := []cameraScanDeviceResponse{}
	for device := range results {
		devices = append(devices, device)
	}
	sort.Slice(devices, func(i, j int) bool {
		if devices[i].IP == devices[j].IP {
			return devices[i].Port < devices[j].Port
		}
		return ipToUint32(net.ParseIP(devices[i].IP).To4()) < ipToUint32(net.ParseIP(devices[j].IP).To4())
	})
	return devices
}

func probeONVIFTarget(ctx context.Context, client *http.Client, ip string, port int) (cameraScanDeviceResponse, bool) {
	paths := []string{"/onvif/device_service", "/onvif/device_service/", "/onvif/DeviceService"}
	for _, path := range paths {
		xaddr := fmt.Sprintf("http://%s:%d%s", ip, port, path)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, xaddr, bytes.NewReader([]byte(onvifDeviceInformationEnvelope)))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")
		req.Header.Set("SOAPAction", "http://www.onvif.org/ver10/device/wsdl/GetDeviceInformation")

		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 128*1024))
		_ = resp.Body.Close()

		if resp.StatusCode == http.StatusUnauthorized {
			return cameraScanDeviceResponse{IP: ip, Port: port, XAddr: xaddr, ONVIF: true, AuthRequired: true, Status: "auth_required"}, true
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			device := cameraScanDeviceResponse{IP: ip, Port: port, XAddr: xaddr, ONVIF: true, Status: "ok"}
			fillONVIFDeviceInformation(body, &device)
			return device, true
		}
		if bytes.Contains(bytes.ToLower(body), []byte("onvif")) || bytes.Contains(bytes.ToLower(body), []byte("getdeviceinformation")) {
			return cameraScanDeviceResponse{IP: ip, Port: port, XAddr: xaddr, ONVIF: true, Status: "onvif_fault"}, true
		}
	}
	return cameraScanDeviceResponse{}, false
}

func fillONVIFDeviceInformation(body []byte, device *cameraScanDeviceResponse) {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	for {
		token, err := decoder.Token()
		if err != nil {
			return
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		var value string
		switch start.Name.Local {
		case "Manufacturer", "Model", "FirmwareVersion", "SerialNumber", "HardwareId", "HardwareID":
			if err := decoder.DecodeElement(&value, &start); err != nil {
				continue
			}
			value = strings.TrimSpace(value)
		default:
			continue
		}
		switch start.Name.Local {
		case "Manufacturer":
			device.Manufacturer = value
		case "Model":
			device.Model = value
		case "FirmwareVersion":
			device.FirmwareVersion = value
		case "SerialNumber":
			device.SerialNumber = value
		case "HardwareId", "HardwareID":
			device.HardwareID = value
		}
	}
}

func isAllowedCameraScanIP(ip net.IP) bool {
	ip = ip.To4()
	if ip == nil {
		return false
	}
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()
}

func (s *Server) annotateExistingScannedCameras(ctx context.Context, devices []cameraScanDeviceResponse) error {
	if len(devices) == 0 {
		return nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, rtsp_url FROM cameras`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type existingCamera struct {
		id   string
		name string
	}
	byHost := map[string]existingCamera{}
	for rows.Next() {
		var id string
		var name string
		var rtspURL string
		if err := rows.Scan(&id, &name, &rtspURL); err != nil {
			return err
		}
		host := hostFromRTSPURL(rtspURL)
		if host == "" {
			continue
		}
		byHost[host] = existingCamera{id: id, name: name}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for index := range devices {
		if existing, ok := byHost[devices[index].IP]; ok {
			devices[index].ExistingCameraID = existing.id
			devices[index].ExistingCameraName = existing.name
		}
	}
	return nil
}

func hostFromRTSPURL(value string) string {
	parsed, err := url.Parse(value)
	if err != nil {
		return ""
	}
	host := parsed.Hostname()
	if net.ParseIP(host) != nil {
		return host
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return host
	}
	for _, ip := range ips {
		if ip.To4() != nil {
			return ip.String()
		}
	}
	return host
}

func ipToUint32(ip net.IP) uint32 {
	return binary.BigEndian.Uint32(ip.To4())
}

func uint32ToIP(value uint32) net.IP {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, value)
	return ip
}

const onvifDeviceInformationEnvelope = `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetDeviceInformation xmlns="http://www.onvif.org/ver10/device/wsdl"/>
  </s:Body>
</s:Envelope>`
