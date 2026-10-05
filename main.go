// Command hikvision-bridge pulls a raw H.264/H.265 stream from a Hikvision
// device over HCNetSDK (private protocol, port 8000) and serves it on a TCP
// socket so go2rtc/ffmpeg can consume it via ffmpeg:tcp://bridge:5000.
//
// Env: HIK_HOST, HIK_PORT (8000), HIK_USER, HIK_PASS,
//
//	HIK_CHANNEL (auto if unset), HIK_STREAM (main|sub|third),
//	HIK_TLS (1|true), LISTEN_ADDR (:5000)
package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/elyor04/go-hikvision-sdk/hikvision"
)

func main() {
	log.SetPrefix("[bridge] ")
	log.SetFlags(0)

	cfg := loadConfig()

	dev, err := hikvision.Login(hikvision.LoginOptions{
		Address:  cfg.host,
		Port:     cfg.port,
		Username: cfg.user,
		Password: cfg.pass,
		UseTLS:   cfg.tls,
	})
	if err != nil {
		log.Fatalf("login %s@%s:%d: %v", cfg.user, cfg.host, cfg.port, err)
	}
	defer dev.Close()
	log.Printf("logged in: %s@%s:%d serial=%s type=%d analog=%d startCh=%d ipCh=%d startIPCh=%d",
		cfg.user, cfg.host, cfg.port, dev.Info.SerialNumber, dev.Info.DeviceType,
		dev.Info.AnalogChannels, dev.Info.StartChannel, dev.Info.IPChannels, dev.Info.StartIPChannel)

	// Channel unset -> pick from device layout (NVR IP channels start at
	// StartIPChannel, e.g. 33; IPC/analog start at StartChannel, usually 1).
	if cfg.channel == 0 {
		switch {
		case dev.Info.AnalogChannels > 0:
			cfg.channel = int32(dev.Info.StartChannel)
		case dev.Info.IPChannels > 0:
			cfg.channel = int32(dev.Info.StartIPChannel)
		default:
			cfg.channel = 1
		}
	}
	log.Printf("using channel=%d stream=%s", cfg.channel, envOr("HIK_STREAM", "main"))

	ln, err := net.Listen("tcp", cfg.listenAddr)
	if err != nil {
		log.Fatalf("listen %s: %v", cfg.listenAddr, err)
	}
	log.Printf("listening on %s (raw H.264/H.265)", cfg.listenAddr)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	// Single persistent RealPlay session shared by sequential clients.
	// Stopping/restarting the SDK session per connect hits SIGBUS inside
	// HCNetSDK teardown under amd64 emulation - keep it alive instead.
	var frames <-chan hikvision.Frame
	openStream := func() {
		frames = nil
		s, err := dev.RealPlay(ctx, cfg.channel, cfg.streamType)
		if err != nil {
			log.Printf("realplay: %v", err)
			return
		}
		frames = s.Frames()
		log.Printf("realplay started")
	}
	openStream()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("accept: %v", err)
			continue
		}
		if frames == nil {
			openStream()
			if frames == nil {
				conn.Close()
				time.Sleep(2 * time.Second)
				continue
			}
		}
		drainStale(frames)
		if serve(conn, frames) {
			frames = nil // channel closed mid-session -> reopen on next client
		}
	}
}

// drainStale drops buffered frames so a fresh client starts near live edge.
func drainStale(frames <-chan hikvision.Frame) {
	var n int
	for {
		select {
		case _, ok := <-frames:
			if !ok {
				return
			}
			n++
		default:
			if n > 0 {
				log.Printf("drained %d stale frames", n)
			}
			return
		}
	}
}

// serve writes stream payloads to conn until the client goes away.
// Returns true if the shared frame channel closed (stream must be reopened).
func serve(conn net.Conn, frames <-chan hikvision.Frame) bool {
	defer conn.Close()
	log.Printf("client connected: %s", conn.RemoteAddr())
	defer log.Printf("client disconnected: %s", conn.RemoteAddr())

	done := make(chan struct{})
	defer close(done)
	var total int64
	types := map[hikvision.StreamDataType]int64{}
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				log.Printf("frames: types=%v sent=%dB", types, total)
			}
		}
	}()
	for frame := range frames {
		types[frame.Type]++
		switch frame.Type {
		case hikvision.StreamStdVideoData, hikvision.StreamData:
		default:
			continue
		}
		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		n, err := conn.Write(frame.Data)
		total += int64(n)
		if err != nil {
			log.Printf("write: %v (sent %d bytes, frame types %v)", err, total, types)
			return false
		}
	}
	log.Printf("frame channel closed, sent %d bytes, frame types %v", total, types)
	return true
}

type config struct {
	host       string
	port       uint16
	user       string
	pass       string
	channel    int32
	streamType hikvision.StreamType
	listenAddr string
	tls        bool
}

func loadConfig() config {
	var cfg config
	cfg.host = os.Getenv("HIK_HOST")
	cfg.user = os.Getenv("HIK_USER")
	cfg.pass = os.Getenv("HIK_PASS")
	if cfg.host == "" || cfg.user == "" || cfg.pass == "" {
		log.Fatal("HIK_HOST, HIK_USER and HIK_PASS are required (see .env.example)")
	}
	cfg.listenAddr = envOr("LISTEN_ADDR", ":5000")
	cfg.tls = envOr("HIK_TLS", "") == "1" || envOr("HIK_TLS", "") == "true"

	port, err := strconv.ParseUint(envOr("HIK_PORT", "8000"), 10, 16)
	if err != nil {
		log.Fatalf("invalid HIK_PORT: %v", err)
	}
	cfg.port = uint16(port)

	channel, err := strconv.ParseInt(envOr("HIK_CHANNEL", "0"), 10, 32)
	if err != nil {
		log.Fatalf("invalid HIK_CHANNEL: %v", err)
	}
	cfg.channel = int32(channel)

	cfg.streamType = hikvision.MainStream
	switch s := envOr("HIK_STREAM", "main"); s {
	case "sub", "substream":
		cfg.streamType = hikvision.SubStream
	case "third":
		cfg.streamType = hikvision.ThirdStream
	case "main":
	default:
		log.Fatalf("invalid HIK_STREAM %q (main|sub|third)", s)
	}
	return cfg
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
