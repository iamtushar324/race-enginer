package ingestion

import (
	"context"

	"github.com/rs/zerolog/log"
	"github.com/tusharbhardwaj/race-engineer/telemetry-core/internal/models"
	"github.com/tusharbhardwaj/race-engineer/telemetry-core/internal/packets"
)

// packetParser is a goroutine that reads raw UDP bytes from packetChan,
// extracts the header to get the packet ID, and forwards the typed
// ParsedPacket for downstream processing.
func packetParser(ctx context.Context, packetChan <-chan []byte, parsedChan chan<- models.ParsedPacket) {
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Packet parser stopping")
			return
		case raw, ok := <-packetChan:
			if !ok {
				return
			}
			if len(raw) < packets.HeaderSize {
				continue
			}

			hdr, err := packets.ParseHeader(raw)
			if err != nil {
				log.Debug().Err(err).Msg("Failed to parse header")
				continue
			}

			parsed := models.ParsedPacket{
				PacketID: hdr.PacketID,
				Data:     raw,
			}

			select {
			case parsedChan <- parsed:
			default:
				log.Warn().Uint8("packet_id", hdr.PacketID).Msg("Parsed channel full, dropping packet")
			}
		}
	}
}
