package main

import (
	"crypto/tls"
	"log"
	"net/http"
	"time"

	"github.com/elazarl/goproxy"
	quic "github.com/lucas-clemente/quic-go"
	http3 "github.com/lucas-clemente/quic-go/http3"
)

var (
	qConf = &quic.Config{
		ConnectionIDLength:             0, // 4 byte(s) — 0 byte for a client, 4-18 byte(s) for a server
		HandshakeIdleTimeout:           10 * time.Second,
		MaxIdleTimeout:                 90 * time.Second,
		MaxIncomingStreams:             10000,
		MaxIncomingUniStreams:          10000,
		InitialStreamReceiveWindow:     10 << 20,  // 10 MB per Stream
		InitialConnectionReceiveWindow: 15 << 20,  // 15 MB per Connection
		MaxStreamReceiveWindow:         100 << 20, // 100 MB per Stream
		MaxConnectionReceiveWindow:     1 << 30,   // 1 GB per Connection
		KeepAlive:                      true,
		DisablePathMTUDiscovery:        false,
		EnableDatagrams:                true,
	}

	tlsConf = &tls.Config{
		InsecureSkipVerify: false,
	}

	h1Tr = &http.Transport{
		DisableKeepAlives:      false,
		DisableCompression:     false,
		ForceAttemptHTTP2:      false,
		TLSClientConfig:        tlsConf,
		TLSHandshakeTimeout:    30 * time.Second,
		ResponseHeaderTimeout:  30 * time.Second,
		IdleConnTimeout:        90 * time.Second,
		ExpectContinueTimeout:  1 * time.Second,
		MaxIdleConns:           1000,     // Prevents resource exhaustion
		MaxIdleConnsPerHost:    100,      // Increases performance and prevents resource exhaustion
		MaxConnsPerHost:        0,        // 0 for no limit
		MaxResponseHeaderBytes: 64 << 10, // 64k
		WriteBufferSize:        64 << 10, // 64k
		ReadBufferSize:         64 << 10, // 64k
	}

	h3Tr = &http3.RoundTripper{
		EnableDatagrams:        true,
		DisableCompression:     false,
		MaxResponseHeaderBytes: 16 << 10, // 16k
		TLSClientConfig:        tlsConf,
		QuicConfig:             qConf,
	}

	h1RoundTripper = goproxy.RoundTripperFunc(func(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Response, error) {
		return h1Tr.RoundTrip(ctx.Req)
	})

	h3RoundTripper = goproxy.RoundTripperFunc(func(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Response, error) {
		return h3Tr.RoundTrip(ctx.Req)
	})
)

func main() {
	proxy := goproxy.NewProxyHttpServer()
	proxy.Verbose = true
	proxy.KeepHeader = true
	proxy.KeepDestinationHeaders = true
	proxy.Tr = h1Tr

	proxy.OnRequest().HandleConnect(handleConnectQuicDialer())
	proxy.OnRequest().DoFunc(
		func(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
			ctx.RoundTripper = h1RoundTripper
			return req, nil
		})
	proxy.OnResponse().DoFunc(
		func(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
			ctx.RoundTripper = h3RoundTripper
			return resp
		})

	log.Fatal(http.ListenAndServe(":7777", proxy))
}

// =========================================
// Handle OnRequest for quicDialer()
func handleConnectQuicDialer() goproxy.HttpsHandler {
	return goproxy.FuncHttpsHandler(func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
		ctx.RoundTripper = h3RoundTripper
		return goproxy.OkConnect, host
	})
}
