package ingestion

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/tusharbhardwaj/race-engineer/telemetry-core/internal/config"
	"github.com/tusharbhardwaj/race-engineer/telemetry-core/internal/models"
	"github.com/tusharbhardwaj/race-engineer/telemetry-core/internal/storage"
)

// Ingester owns the full ingestion pipeline: UDP listener → parser → state writer.
// All three stages run as self-healing goroutines.
type Ingester struct {
	cfg        *config.Config
	store      *storage.Storage
	packetChan chan []byte
	parsedChan chan models.ParsedPacket
	packetsRx  atomic.Uint64 // total packets received, for health endpoint
	PTTChan    chan bool      // push-to-talk state changes from F1 BUTN events
}

// NewIngester creates a new Ingester wired to the given config and storage.
func NewIngester(cfg *config.Config, store *storage.Storage) *Ingester {
	return &Ingester{
		cfg:        cfg,
		store:      store,
		packetChan: make(chan []byte, 4096),
		parsedChan: make(chan models.ParsedPacket, 2048),
		PTTChan:    make(chan bool, 16),
	}
}

// Start launches the three pipeline goroutines into the provided WaitGroup.
func (ing *Ingester) Start(ctx context.Context, wg *sync.WaitGroup) {
	wg.Add(3)
	go selfHeal(ctx, wg, "udp-listener", func(c context.Context) {
		for {
			select {
			case <-c.Done():
				return
			default:
			}
			if ing.cfg.MockMode.Load() {
				// Use the enhanced mock generator (all 16 packet types).
				mock := &MockGenerator{}
				mock.Run(c, ing.cfg, ing.packetChan, &ing.packetsRx, ing.cfg.MockMode.Load)
			} else {
				ing.udpLoop(c)
			}
		}
	})
	go selfHeal(ctx, wg, "packet-parser", func(c context.Context) {
		packetParser(c, ing.packetChan, ing.parsedChan)
	})
	go selfHeal(ctx, wg, "state-writer", func(c context.Context) {
		stateWriter(c, ing.parsedChan, ing.store, ing.cfg.SampleRate, ing.cfg.PTTButton, ing.PTTChan)
	})

	log.Info().
		Bool("mock", ing.cfg.MockMode.Load()).
		Int("udp_port", ing.cfg.UDPPort).
		Msg("Ingestion pipeline started (3 goroutines)")
}

// PacketsReceived returns the total number of UDP packets received.
func (ing *Ingester) PacketsReceived() uint64 {
	return ing.packetsRx.Load()
}

// udpLoop binds to the UDP port and reads packets in a tight loop.
// It watches RestartGen to rebind when the host/port changes at runtime.
//
// Two modes:
//   - broadcast: PS5/game broadcasts to all devices. Bind 0.0.0.0 with SO_BROADCAST.
//   - unicast:   PS5/game sends to this Mac's IP. Bind to that specific IP.
func (ing *Ingester) udpLoop(ctx context.Context) {
	host := ing.cfg.RuntimeUDPHost()
	port := ing.cfg.RuntimeUDPPort()
	udpMode := ing.cfg.RuntimeUDPMode()

	// Decide bind address based on mode.
	bindHost := host // unicast: bind to the Mac's IP so only targeted packets arrive
	if udpMode == "broadcast" {
		bindHost = "0.0.0.0" // broadcast: listen on all interfaces
	}

	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", bindHost, port))
	if err != nil {
		panic(fmt.Errorf("failed to resolve UDP address %s:%d: %v", bindHost, port, err))
	}

	// Use ListenConfig with Control to set socket options before bind.
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) {
				_ = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
				_ = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEPORT, 1)
				if udpMode == "broadcast" {
					_ = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
				}
			})
		},
	}

	pc, err := lc.ListenPacket(ctx, "udp", addr.String())
	if err != nil {
		panic(fmt.Errorf("failed to bind UDP socket in %s mode on %s: %v", udpMode, addr.String(), err))
	}
	conn := pc.(*net.UDPConn)
	defer conn.Close()

	// 2 MB kernel read buffer for burst absorption.
	_ = conn.SetReadBuffer(2 * 1024 * 1024)

	log.Info().Str("mode", udpMode).Str("bind", addr.String()).Str("host_setting", host).Msg("UDP listener bound")

	buf := make([]byte, 2048) // F1 packets are always < 1500 bytes.
	gen := ing.cfg.RestartGen.Load()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// If config changed (host/port/mode), exit so selfHeal restarts us.
		if ing.cfg.RestartGen.Load() != gen {
			log.Info().Msg("UDP config changed, restarting listener")
			return
		}

		// Short deadline so we check ctx every 500ms.
		_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			log.Warn().Err(err).Msg("UDP read error")
			continue
		}

		ing.packetsRx.Add(1)

		// Copy to avoid overwrite on next read.
		pkt := make([]byte, n)
		copy(pkt, buf[:n])

		select {
		case ing.packetChan <- pkt:
		default:
			log.Warn().Msg("Packet channel full, dropping UDP packet")
		}
	}
}
