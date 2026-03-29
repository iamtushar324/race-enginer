package intelligence

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/tusharbhardwaj/race-engineer/telemetry-core/internal/models"
)

// AudioBroadcaster is the interface for pushing audio to WebSocket clients.
// Implemented by api.Hub.
type AudioBroadcaster interface {
	BroadcastAudio(audioBase64 string, format string)
}

// VoiceClient drains a channel of translated insights, synthesizes them into
// audio via the Python voice service at /synthesize, and broadcasts the
// base64-encoded audio to all WebSocket clients.
type VoiceClient struct {
	voiceURL  string
	client    *http.Client
	voiceChan <-chan models.DrivingInsight
	hub       AudioBroadcaster
}

// synthesizePayload is the JSON body sent to the Python voice service.
type synthesizePayload struct {
	Text     string `json:"text"`
	Priority int    `json:"priority"`
}

// NewVoiceClient creates a voice client that synthesizes audio via the Python
// voice service and broadcasts it over WebSocket.
func NewVoiceClient(voiceURL string, voiceChan <-chan models.DrivingInsight, hub AudioBroadcaster) *VoiceClient {
	return &VoiceClient{
		voiceURL: voiceURL,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
		voiceChan: voiceChan,
		hub:       hub,
	}
}

// Run drains the voice channel, synthesizing each insight into audio.
// Blocks until ctx is cancelled.
func (vc *VoiceClient) Run(ctx context.Context) {
	log.Info().Str("url", vc.voiceURL).Msg("Voice client started")
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Voice client stopping")
			return
		case insight, ok := <-vc.voiceChan:
			if !ok {
				return
			}
			vc.synthesize(ctx, insight)
		}
	}
}

// SynthesizeAck sends a request to the voice service to play a short random
// acknowledgement sound (e.g. "Copy that", "Roger"). Designed to be called
// in a goroutine for instant feedback before the LLM responds.
func (vc *VoiceClient) SynthesizeAck(ctx context.Context) {
	url := fmt.Sprintf("%s/synthesize-ack", vc.voiceURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to create ack request")
		return
	}

	resp, err := vc.client.Do(req)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to POST /synthesize-ack")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		log.Warn().Int("status", resp.StatusCode).Msg("Voice service /synthesize-ack non-success")
		return
	}

	audioBytes, err := io.ReadAll(resp.Body)
	if err != nil || len(audioBytes) == 0 {
		return
	}

	format := "mp3"
	ct := resp.Header.Get("Content-Type")
	if ct == "audio/wav" || ct == "audio/x-wav" {
		format = "wav"
	}

	encoded := base64.StdEncoding.EncodeToString(audioBytes)
	vc.hub.BroadcastAudio(encoded, format)
	log.Debug().Int("audio_bytes", len(audioBytes)).Msg("Ack audio broadcast")
}

// synthesize POSTs insight text to the Python voice service /synthesize endpoint,
// receives raw audio bytes (audio/mpeg), base64-encodes them, and broadcasts
// to all WebSocket clients via the hub.
func (vc *VoiceClient) synthesize(ctx context.Context, insight models.DrivingInsight) {
	payload := synthesizePayload{
		Text:     insight.Message,
		Priority: insight.Priority,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal synthesize payload")
		return
	}

	url := fmt.Sprintf("%s/synthesize", vc.voiceURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		log.Error().Err(err).Msg("Failed to create synthesize request")
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := vc.client.Do(req)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to POST to voice service /synthesize")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		log.Warn().Int("status", resp.StatusCode).Str("message", insight.Message).Msg("Voice service /synthesize returned non-success")
		return
	}

	// Read raw audio bytes from response.
	audioBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read audio response body")
		return
	}

	if len(audioBytes) == 0 {
		log.Warn().Msg("Voice service returned empty audio")
		return
	}

	// Detect audio format from Content-Type header.
	format := "mp3"
	ct := resp.Header.Get("Content-Type")
	if ct == "audio/wav" || ct == "audio/x-wav" {
		format = "wav"
	}

	// Base64-encode and broadcast to WebSocket clients.
	encoded := base64.StdEncoding.EncodeToString(audioBytes)
	vc.hub.BroadcastAudio(encoded, format)

	log.Debug().
		Str("type", insight.Type).
		Int("priority", insight.Priority).
		Int("audio_bytes", len(audioBytes)).
		Msg("Audio synthesized and broadcast to WebSocket clients")
}
