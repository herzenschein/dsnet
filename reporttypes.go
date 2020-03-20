package dsnet

import (
	"encoding/json"
	"io/ioutil"
	"net"
	"os"
	"time"

	"github.com/go-playground/validator/v10"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type Status int

const (
	// Host has not been loaded into wireguard yet
	StatusUnknown = iota
	// No handshake in 3 minutes
	StatusOffline
	// Handshake in 3 minutes
	StatusOnline
	// Host has not connected for 28 days and may be removed
	StatusDormant
)

func (s Status) String() string {
	switch s {
	case StatusUnknown:
		return "unknown"
	case StatusOffline:
		return "offline"
	case StatusOnline:
		return "online"
	case StatusDormant:
		return "dormant"
	default:
		return "";
	}
}

// note unmarshal not required
func (s Status) MarshalJSON() ([]byte, error) {
	return []byte("\"" + s.String() + "\""), nil
}

type DsnetReport struct {
	ExternalIP    net.IP
	InterfaceName string
	ListenPort    int
	// domain to append to hostnames. Relies on separate DNS server for
	// resolution. Informational only.
	Domain string
	IP     net.IP
	// IP network from which to allocate automatic sequential addresses
	// Network is chosen randomly when not specified
	Network     JSONIPNet
	DNS         net.IP
	PeersOnline int
	PeersTotal  int
	Peers       []PeerReport
}

func GenerateReport(dev *wgtypes.Device, conf *DsnetConfig, oldReport *DsnetReport) DsnetReport {
	wgPeerIndex := make(map[wgtypes.Key]wgtypes.Peer)
	peerReports := make([]PeerReport, len(conf.Peers))
	oldPeerReportIndex := make(map[string]PeerReport)
	peersOnline := 0

	for _, peer := range dev.Peers {
		wgPeerIndex[peer.PublicKey] = peer
	}

	if oldReport != nil {
		for _, report := range oldReport.Peers {
			oldPeerReportIndex[report.Hostname] = report
		}
	}

	for i, peer := range conf.Peers {
		wgPeer, known := wgPeerIndex[peer.PublicKey.Key]

		status := Status(StatusUnknown)

		if !known {
			status = StatusUnknown
		} else if time.Since(wgPeer.LastHandshakeTime) < TIMEOUT {
			status = StatusOnline
			peersOnline += 1
		} else if !wgPeer.LastHandshakeTime.IsZero() && time.Since(wgPeer.LastHandshakeTime) > EXPIRY {
			status = StatusDormant
		} else {
			status = StatusOffline
		}

		externalIP := net.IP{}
		if wgPeer.Endpoint != nil {
			externalIP = wgPeer.Endpoint.IP
		}

		peerReports[i] = PeerReport{
			Hostname:          peer.Hostname,
			Owner:             peer.Owner,
			Description:       peer.Description,
			Added:             peer.Added,
			IP:                peer.IP,
			ExternalIP:        externalIP,
			Status:            status,
			Networks:          peer.Networks,
			LastHandshakeTime: wgPeer.LastHandshakeTime,
			ReceiveBytes:      wgPeer.ReceiveBytes,
			TransmitBytes:     wgPeer.TransmitBytes,
			ReceiveBytesSI:    BytesToSI(wgPeer.ReceiveBytes),
			TransmitBytesSI:   BytesToSI(wgPeer.TransmitBytes),
		}
	}

	return DsnetReport{
		ExternalIP:    conf.ExternalIP,
		InterfaceName: conf.InterfaceName,
		ListenPort:    conf.ListenPort,
		Domain:        conf.Domain,
		IP:            conf.IP,
		Network:       conf.Network,
		DNS:           conf.DNS,
		Peers:         peerReports,
		PeersOnline:   peersOnline,
		PeersTotal:    len(peerReports),
	}
}

func (report *DsnetReport) MustSave(filename string) {
	_json, _ := json.MarshalIndent(report, "", "    ")
	err := ioutil.WriteFile(filename, _json, 0644)
	check(err)
}

func MustLoadDsnetReport() *DsnetReport {
	raw, err := ioutil.ReadFile(CONFIG_FILE)

	if os.IsNotExist(err) {
		return nil
	} else if os.IsPermission(err) {
		ExitFail("%s cannot be accessed. Check read permissions.", CONFIG_FILE)
	} else {
		check(err)
	}

	report := DsnetReport{}
	err = json.Unmarshal(raw, &report)
	check(err)

	err = validator.New().Struct(report)
	check(err)

	return &report
}

type PeerReport struct {
	// Used to update DNS
	Hostname string
	// username of person running this host/router
	Owner string
	// Description of what the host is and/or does
	Description string
	// date peer was added to dsnet config
	Added time.Time
	// Internal VPN IP address. Added to AllowedIPs in server config as a /32
	IP net.IP
	// Last known external IP
	ExternalIP net.IP
	Status     Status
	// TODO ExternalIP support (Endpoint)
	//ExternalIP     net.UDPAddr `validate:"required,udp4_addr"`
	// TODO support routing additional networks (AllowedIPs)
	Networks          []JSONIPNet
	LastHandshakeTime time.Time
	ReceiveBytes      int64
	TransmitBytes     int64
	ReceiveBytesSI    string
	TransmitBytesSI   string
}
