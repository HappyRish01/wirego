package pkg

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// TODO: Replace with your deployed Vercel API URL
const SignalingServerURL = "https://wirego-signalling.vercel.app/api"

type SignalingClient struct {
	client  *http.Client
	baseURL string
	code    string
}

type CreateRequest struct {
	SDP string `json:"sdp"`
}

type CreateResponse struct {
	Code string `json:"code"`
}

type AnswerRequest struct {
	Code string `json:"code"`
	SDP  string `json:"sdp"`
}

type GetResponse struct {
	SDP   string `json:"sdp"`
	Found bool   `json:"found"`
}

func NewSignalingClient(code string) *SignalingClient {
	transport := &http.Transport{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     30 * time.Second,
		DisableCompression:  true, // we already compress SDP
	}
	return &SignalingClient{
		client: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
		baseURL: SignalingServerURL,
		code:    code,
	}
}

// CompressSDP compresses and base64-encodes the SDP
func CompressSDP(sdp string) (string, error) {
	var buf bytes.Buffer
	// gz := gzip.NewWriter(&buf)
	gz, _ := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	if _, err := gz.Write([]byte(sdp)); err != nil {
		return "", err
	}
	if err := gz.Close(); err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(buf.Bytes()), nil
}

// DecompressSDP decodes and decompresses the SDP
func DecompressSDP(compressed string) (string, error) {
	data, err := base64.RawStdEncoding.DecodeString(compressed)
	if err != nil {
		return "", err
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	defer gz.Close()

	decoded, err := io.ReadAll(gz)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

// CreateOffer sends the offer SDP and gets back a code
func (s *SignalingClient) CreateOffer(sdp string) (string, error) {
	compressed, err := CompressSDP(sdp)
	if err != nil {
		return "", fmt.Errorf("failed to compress SDP: %w", err)
	}

	reqBody := CreateRequest{SDP: compressed}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	resp, err := s.client.Post(s.baseURL+"/create", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to connect to signaling server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	var createResp CreateResponse
	if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
		return "", err
	}

	s.code = createResp.Code
	return createResp.Code, nil
}

// GetOffer fetches the offer SDP using a code
func (s *SignalingClient) GetOffer(code string) (string, error) {
	s.code = code

	resp, err := s.client.Get(fmt.Sprintf("%s/get?code=%s", s.baseURL, code))
	if err != nil {
		return "", fmt.Errorf("failed to fetch offer: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("code not found - check if sender is online")
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	var getResp GetResponse
	if err := json.NewDecoder(resp.Body).Decode(&getResp); err != nil {
		return "", err
	}

	if !getResp.Found {
		return "", fmt.Errorf("offer not found")
	}

	return DecompressSDP(getResp.SDP)
}

// SendAnswer sends the answer SDP back using the code
func (s *SignalingClient) SendAnswer(sdp string) error {
	compressed, err := CompressSDP(sdp)
	if err != nil {
		return fmt.Errorf("failed to compress SDP: %w", err)
	}

	reqBody := AnswerRequest{
		Code: s.code,
		SDP:  compressed,
	}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	resp, err := s.client.Post(s.baseURL+"/answer", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to send answer: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// WaitForAnswer polls for the answer SDP
func (s *SignalingClient) WaitForAnswer() (string, error) {
	maxAttempts := 60 // 60 attempts * 1 second = 1 minute timeout
	delay := 500 * time.Millisecond
	maxDelay := 3 * time.Second

	for i := 0; i < maxAttempts; i++ {
		resp, err := s.client.Get(fmt.Sprintf("%s/answer?code=%s", s.baseURL, s.code))
		if err != nil {
			// time.Sleep(1 * time.Second)
			time.Sleep(delay)
			if delay < maxDelay {
				delay *= 2
			}
			continue
		}

		if resp.StatusCode == http.StatusOK {
			var getResp GetResponse
			// if err := json.NewDecoder(resp.Body).Decode(&getResp); err != nil {
			// 	resp.Body.Close()
			// 	return "", err
			// }
			dec := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)) // 1MB cap
			dec.DisallowUnknownFields()
			err := dec.Decode(&getResp)

			if err != nil {
				return "", err
			}
			resp.Body.Close()

			if getResp.Found {
				return DecompressSDP(getResp.SDP)
			}
		} else {
			resp.Body.Close()
		}

		time.Sleep(1 * time.Second)
	}

	return "", fmt.Errorf("timeout waiting for answer")
}

func (s *SignalingClient) Close() error {
	// No persistent connection to close with HTTP
	return nil
}
