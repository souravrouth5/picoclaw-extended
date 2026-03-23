package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

func init() {
	// Skip custom DNS if /etc/resolv.conf exists and is non-empty (functional system DNS).
	// On Termux/Android, /etc/resolv.conf may exist but be empty or non-functional,
	// so we check the file size rather than just existence.
	if fi, err := os.Stat("/etc/resolv.conf"); err == nil && fi.Size() > 0 {
		return
	}

	// 从环境变量获取 DNS server 列表，多个用 ; 隔开
	// 例如: PICOCLAW_DNS_SERVER="8.8.8.8:53;1.1.1.1:53;223.5.5.5:53"
	dnsEnv := os.Getenv("PICOCLAW_DNS_SERVER")
	if dnsEnv == "" {
		dnsEnv = "8.8.8.8:53;1.1.1.1:53"
	}

	var dnsServers []string
	for _, s := range strings.Split(dnsEnv, ";") {
		s = strings.TrimSpace(s)
		if s != "" {
			// 如果没有带端口号，自动补上 :53
			if _, _, err := net.SplitHostPort(s); err != nil {
				s = s + ":53"
			}
			dnsServers = append(dnsServers, s)
		}
	}

	// 轮询索引，在多个 DNS 服务器之间轮转
	var idx uint64

	customResolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 5 * time.Second}
			// Round-robin: 依次尝试不同的 DNS 服务器
			server := dnsServers[atomic.AddUint64(&idx, 1)%uint64(len(dnsServers))]
			return d.DialContext(ctx, "udp", server)
		},
	}

	// 覆盖全局 DefaultResolver
	net.DefaultResolver = customResolver

	// 覆盖 http.DefaultTransport 使用自定义 DNS 解析的 DialContext
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		Resolver:  customResolver,
	}

	if tr, ok := http.DefaultTransport.(*http.Transport); ok {
		tr.DialContext = dialer.DialContext
	}
}
