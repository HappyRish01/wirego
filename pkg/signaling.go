package pkg

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// const SignalingServerURL = "https://wirego-signalling.vercel.app/api"
const SignalingServerURL = "https://w-signal.vercel.app/api"

const (
	maxResponseSize = 1 << 20 // 1MB limit to prevent memory exhaustion
	maxRetries      = 60      // maximum polling attempts
	initialDelay    = 500 * time.Millisecond
	maxDelay        = 3 * time.Second
	requestTimeout  = 30 * time.Second
)

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
	gz, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	if err != nil {
		return "", fmt.Errorf("failed to create gzip writer: %w", err)
	}

	if _, err := gz.Write([]byte(sdp)); err != nil {
		return "", fmt.Errorf("failed to compress data: %w", err)
	}

	if err := gz.Close(); err != nil {
		return "", fmt.Errorf("failed to close gzip writer: %w", err)
	}

	return base64.RawStdEncoding.EncodeToString(buf.Bytes()), nil
}

// DecompressSDP decodes and decompresses the SDP
func DecompressSDP(compressed string) (string, error) {
	data, err := base64.RawStdEncoding.DecodeString(compressed)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gz.Close()

	decoded, err := io.ReadAll(io.LimitReader(gz, maxResponseSize))
	if err != nil {
		return "", fmt.Errorf("failed to decompress data: %w", err)
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
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	var createResp CreateResponse
	dec := json.NewDecoder(io.LimitReader(resp.Body, maxResponseSize))
	if err := dec.Decode(&createResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
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
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	var getResp GetResponse
	dec := json.NewDecoder(io.LimitReader(resp.Body, maxResponseSize))
	if err := dec.Decode(&getResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
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
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// WaitForAnswer polls for the answer SDP
func (s *SignalingClient) WaitForAnswer() (string, error) {
	return s.WaitForAnswerWithContext(context.Background())
}

// WaitForAnswerWithContext polls for the answer SDP with cancellation support
func (s *SignalingClient) WaitForAnswerWithContext(ctx context.Context) (string, error) {
	delay := initialDelay

	for i := 0; i < maxRetries; i++ {
		// Check if context is cancelled (Ctrl+C support)
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("operation cancelled: %w", ctx.Err())
		default:
		}

		resp, err := s.client.Get(fmt.Sprintf("%s/answer?code=%s", s.baseURL, s.code))
		if err != nil {
			// Network error - use exponential backoff before retry
			time.Sleep(delay)
			if delay < maxDelay {
				delay *= 2
			}
			continue
		}

		// Always close response body to prevent resource leak
		var result string
		func() {
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				var getResp GetResponse
				dec := json.NewDecoder(io.LimitReader(resp.Body, maxResponseSize))
				dec.DisallowUnknownFields()

				if err := dec.Decode(&getResp); err != nil {
					// JSON decode error - continue trying
					return
				}

				if getResp.Found {
					decompressed, err := DecompressSDP(getResp.SDP)
					if err == nil {
						result = decompressed
					}
				}
			}
			// Non-200 responses are expected (answer not ready yet)
		}()

		// Check if we got a successful result
		if result != "" {
			return result, nil
		}

		// Wait before next poll
		time.Sleep(1 * time.Second)
	}

	return "", fmt.Errorf("timeout waiting for answer after %d attempts", maxRetries)
}

func (s *SignalingClient) Close() error {
	// Close idle connections to free resources
	s.client.CloseIdleConnections()
	return nil
}
