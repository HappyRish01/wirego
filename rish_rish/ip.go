package pkg

import (
	"sync"

	"github.com/pion/webrtc/v3"
)

// finds the public IP addresses accociated with the device by calling Googles STUN servers.
func FindPublicIP() ([]string, error) {
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}
	peerConnection, err := webrtc.NewPeerConnection(config)
	if err != nil {
		return nil, err
	}
	defer peerConnection.Close()

	var (
		mu      sync.Mutex
		address []string
		seen    = make(map[string]struct{})
	)

	peerConnection.OnICECandidate(func(i *webrtc.ICECandidate) {
		// cause we want only public IPs
		if i != nil && i.Typ == webrtc.ICECandidateTypeSrflx {
			mu.Lock()
			defer mu.Unlock()

			// not taking duplicate IPs

			if _, exists := seen[i.Address]; !exists {
				seen[i.Address] = struct{}{}
				address = append(address, i.Address)
			}
		}
	})

	gatherDone := webrtc.GatheringCompletePromise(peerConnection)
	offer, err := peerConnection.CreateOffer(nil)
	if err != nil {
		return nil, err
	}

	// this calls the STUN server.
	err = peerConnection.SetLocalDescription(offer)
	if err != nil {
		return nil, err
	}

	// wait for receiving all the ICECandidates.
	<-gatherDone
	return address, nil
}
