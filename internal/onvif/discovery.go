package onvif

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"maps"
	"net"
	"net/url"
	"slices"
	"strings"
	"time"
)

const (
	probeWaitDuration = 3 * time.Second
)

// Camera is a network video transmitter discovered through ONVIF.
type Camera struct {
	Name       string `json:"name"`
	Address    string `json:"address"`
	ServiceURL string `json:"serviceURL"`
}

type probeEnvelope struct {
	Body struct {
		ProbeMatches struct {
			Matches []struct {
				Scopes string `xml:"Scopes"`
				XAddrs string `xml:"XAddrs"`
			} `xml:"ProbeMatch"`
		} `xml:"ProbeMatches"`
	} `xml:"Body"`
}

// DiscoverCameras asks the local network for ONVIF transmitters and gathers
// their device service addresses until the discovery window closes.
func DiscoverCameras() ([]Camera, error) {
	connection, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero})
	if err != nil {
		return nil, fmt.Errorf("opening the local discovery socket: %w", err)
	}
	defer connection.Close()

	messageID, err := newMessageID()
	if err != nil {
		return nil, fmt.Errorf("creating the discovery request: %w", err)
	}

	if err := connection.SetDeadline(time.Now().Add(probeWaitDuration)); err != nil {
		return nil, fmt.Errorf("setting the discovery timeout: %w", err)
	}

	if _, err := connection.WriteToUDP([]byte(probeMessage(messageID)), &net.UDPAddr{
		IP:   net.ParseIP("239.255.255.250"),
		Port: 3702,
	}); err != nil {
		return nil, fmt.Errorf("searching the local network: %w", err)
	}

	found := make(map[string]Camera)
	packet := make([]byte, 64*1024)
	for {
		packetSize, _, readError := connection.ReadFromUDP(packet)
		if readError != nil {
			if timeout, ok := readError.(net.Error); ok && timeout.Timeout() {
				break
			}
			return nil, fmt.Errorf("reading camera discovery replies: %w", readError)
		}

		for _, camera := range parseProbeMatches(packet[:packetSize]) {
			found[camera.ServiceURL] = camera
		}
	}

	serviceAddresses := slices.Sorted(maps.Keys(found))

	cameras := make([]Camera, 0, len(serviceAddresses))
	for _, serviceAddress := range serviceAddresses {
		cameras = append(cameras, found[serviceAddress])
	}

	return cameras, nil
}

func newMessageID() (string, error) {
	var identifier [16]byte
	if _, err := rand.Read(identifier[:]); err != nil {
		return "", err
	}

	encoded := hex.EncodeToString(identifier[:])
	return "urn:uuid:" + encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:], nil
}

func probeMessage(messageID string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
 xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing"
 xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery"
 xmlns:dn="http://www.onvif.org/ver10/network/wsdl">
 <s:Header>
  <a:MessageID>%s</a:MessageID>
  <a:To>urn:schemas-xmlsoap-org:ws:2005:04:discovery</a:To>
  <a:Action>http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe</a:Action>
 </s:Header>
 <s:Body><d:Probe><d:Types>dn:NetworkVideoTransmitter</d:Types></d:Probe></s:Body>
</s:Envelope>`, messageID)
}

func parseProbeMatches(message []byte) []Camera {
	var response probeEnvelope
	if err := xml.Unmarshal(message, &response); err != nil {
		return nil
	}

	cameras := make([]Camera, 0, len(response.Body.ProbeMatches.Matches))
	for _, match := range response.Body.ProbeMatches.Matches {
		serviceAddress := firstServiceAddress(match.XAddrs)
		if serviceAddress == "" {
			continue
		}

		parsedAddress, err := url.Parse(serviceAddress)
		if err != nil || parsedAddress.Hostname() == "" {
			continue
		}

		name := cameraName(match.Scopes)
		if name == "" {
			name = parsedAddress.Hostname()
		}

		cameras = append(cameras, Camera{
			Name:       name,
			Address:    parsedAddress.Hostname(),
			ServiceURL: parsedAddress.String(),
		})
	}

	return cameras
}

func firstServiceAddress(addresses string) string {
	for _, address := range strings.Fields(addresses) {
		parsedAddress, err := url.Parse(address)
		if err != nil || parsedAddress.Hostname() == "" {
			continue
		}
		if parsedAddress.Scheme == "http" || parsedAddress.Scheme == "https" {
			return parsedAddress.String()
		}
	}

	return ""
}

func cameraName(scopes string) string {
	const nameScopePrefix = "onvif://www.onvif.org/name/"

	for _, scope := range strings.Fields(scopes) {
		if !strings.HasPrefix(scope, nameScopePrefix) {
			continue
		}

		name, err := url.PathUnescape(strings.TrimPrefix(scope, nameScopePrefix))
		if err == nil && name != "" {
			return name
		}
	}

	return ""
}
