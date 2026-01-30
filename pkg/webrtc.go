package pkg

import (
	"fmt"
	"io"

	"github.com/pion/webrtc/v3"
)

type WebRTCConnection struct {
	PC        *webrtc.PeerConnection // Exported for external use
	dc        *webrtc.DataChannel
	onMessage chan []byte
	onOpen    chan struct{}
	onClose   chan struct{}
}

func NewWebRTCConnection() (*WebRTCConnection, error) {
	// Optimized configuration for large file transfers
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{
					"stun:stun.l.google.com:19302",
					"stun:stun1.l.google.com:19302",
					"stun:stun2.l.google.com:19302",
				},
			},
		},
	}

	// Create peer connection with settings engine for better performance
	settingsEngine := webrtc.SettingEngine{}

	// Increase buffer sizes for large transfers
	settingsEngine.SetSCTPMaxReceiveBufferSize(16 * 1024 * 1024) // 16MB

	api := webrtc.NewAPI(webrtc.WithSettingEngine(settingsEngine))

	pc, err := api.NewPeerConnection(config)
	if err != nil {
		return nil, err
	}

	return &WebRTCConnection{
		PC:        pc,
		onMessage: make(chan []byte, 100),
		onOpen:    make(chan struct{}),
		onClose:   make(chan struct{}),
	}, nil
}

func (w *WebRTCConnection) CreateOffer() (string, error) {
	// Create data channel
	dc, err := w.PC.CreateDataChannel("filetransfer", nil)
	if err != nil {
		return "", err
	}
	w.dc = dc

	// Setup handlers
	dc.OnOpen(func() {
		close(w.onOpen)
	})

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		w.onMessage <- msg.Data
	})

	dc.OnClose(func() {
		close(w.onClose)
	})

	// Create offer
	offer, err := w.PC.CreateOffer(nil)
	if err != nil {
		return "", err
	}

	err = w.PC.SetLocalDescription(offer)
	if err != nil {
		return "", err
	}

	// Wait for ICE gathering
	<-webrtc.GatheringCompletePromise(w.PC)

	return w.PC.LocalDescription().SDP, nil
}

func (w *WebRTCConnection) SetAnswer(sdp string) error {
	answer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  sdp,
	}
	return w.PC.SetRemoteDescription(answer)
}

func (w *WebRTCConnection) CreateAnswer(offerSDP string) (string, error) {
	// Set remote offer
	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  offerSDP,
	}

	err := w.PC.SetRemoteDescription(offer)
	if err != nil {
		return "", err
	}

	// Setup data channel handler
	w.PC.OnDataChannel(func(dc *webrtc.DataChannel) {
		w.dc = dc

		dc.OnOpen(func() {
			close(w.onOpen)
		})

		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			w.onMessage <- msg.Data
		})

		dc.OnClose(func() {
			close(w.onClose)
		})
	})

	// Create answer
	answer, err := w.PC.CreateAnswer(nil)
	if err != nil {
		return "", err
	}

	err = w.PC.SetLocalDescription(answer)
	if err != nil {
		return "", err
	}

	// Wait for ICE gathering
	<-webrtc.GatheringCompletePromise(w.PC)

	return w.PC.LocalDescription().SDP, nil
}

func (w *WebRTCConnection) Send(data []byte) error {
	if w.dc == nil {
		return fmt.Errorf("data channel not ready")
	}
	return w.dc.Send(data)
}

func (w *WebRTCConnection) Receive() ([]byte, error) {
	select {
	case data, ok := <-w.onMessage:
		if !ok {
			return nil, io.EOF
		}
		return data, nil
	case <-w.onClose:
		return nil, io.EOF
	}
}

func (w *WebRTCConnection) WaitForOpen() {
	<-w.onOpen
}

func (w *WebRTCConnection) Close() error {
	if w.dc != nil {
		w.dc.Close()
	}
	return w.PC.Close()
}
